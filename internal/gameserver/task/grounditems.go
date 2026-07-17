package task

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// GroundItemTick is the cleanup cadence for dropped ground items.
const GroundItemTick = 5 * time.Second

// GroundItemOptions controls how long dropped items stay in the world.
type GroundItemOptions struct {
	HerbAutoDestroy      time.Duration
	ItemAutoDestroy      time.Duration
	EquipableAutoDestroy time.Duration
	SpecialAutoDestroy   map[int32]time.Duration

	PlayerDroppedMultiplier int
}

// DefaultGroundItemOptions returns the shipped server.properties defaults.
func DefaultGroundItemOptions() GroundItemOptions {
	return GroundItemOptions{
		HerbAutoDestroy:         15 * time.Second,
		ItemAutoDestroy:         600 * time.Second,
		EquipableAutoDestroy:    0,
		SpecialAutoDestroy:      map[int32]time.Duration{57: 0, 5575: 0, 6673: 0},
		PlayerDroppedMultiplier: 1,
	}
}

// GroundItemOptionsFromProperties reads the item cleanup settings from
// server.properties.
func GroundItemOptionsFromProperties(props *config.Properties) (GroundItemOptions, error) {
	opts := DefaultGroundItemOptions()
	if props == nil {
		return opts, nil
	}

	f := config.NewFields(props, "ground item options")
	herb := f.Int("AutoDestroyHerbTime", int(opts.HerbAutoDestroy/time.Second))
	regular := f.Int("AutoDestroyItemTime", int(opts.ItemAutoDestroy/time.Second))
	equipable := f.Int("AutoDestroyEquipableItemTime", int(opts.EquipableAutoDestroy/time.Second))
	multiplier := f.Int("PlayerDroppedItemMultiplier", opts.PlayerDroppedMultiplier)
	pairs := f.IntPairs("AutoDestroySpecialItemTime", "57-0,5575-0,6673-0")
	if err := f.Err(); err != nil {
		return GroundItemOptions{}, err
	}

	special := make(map[int32]time.Duration, len(pairs))
	for _, pair := range pairs {
		if pair.First < math.MinInt32 || pair.First > math.MaxInt32 {
			return GroundItemOptions{}, fmt.Errorf("AutoDestroySpecialItemTime: item id %d overflows int32", pair.First)
		}
		special[int32(pair.First)] = time.Duration(pair.Second) * time.Second
	}

	opts.HerbAutoDestroy = time.Duration(herb) * time.Second
	opts.ItemAutoDestroy = time.Duration(regular) * time.Second
	opts.EquipableAutoDestroy = time.Duration(equipable) * time.Second
	opts.PlayerDroppedMultiplier = multiplier
	opts.SpecialAutoDestroy = special
	return opts, nil
}

// DropOptions describes where and how an item was dropped.
type DropOptions struct {
	X, Y, Z, Heading int
	PlayerDropped    bool

	// DropperID is the object id of the creature the item fell from (a
	// player discarding an item, or a defeated NPC dropping loot). It is
	// zero for items with no fall animation, e.g. restored ground items.
	DropperID int32
}

type groundItemEntry struct {
	item      *grounditem.Item
	expiresAt time.Time
}

// GroundItems owns dropped ground items and their cleanup deadlines.
//
// mu guards items. Item positions and visibility are guarded by each item's
// embedded world.Presence.
type GroundItems struct {
	state *world.State
	now   func() time.Time
	opts  GroundItemOptions

	mu    sync.RWMutex
	items map[int32]groundItemEntry
}

// NewGroundItems returns an empty ground-item owner.
func NewGroundItems(state *world.State, opts GroundItemOptions, now func() time.Time) *GroundItems {
	if state == nil {
		state = world.New()
	}
	if now == nil {
		now = time.Now
	}
	return &GroundItems{
		state: state,
		now:   now,
		opts:  opts,
		items: make(map[int32]groundItemEntry),
	}
}

// Start launches the fixed ground-item cleanup task.
func (g *GroundItems) Start(log zerolog.Logger) *scheduler.Ticker {
	return scheduler.Start(GroundItemTick, g.Tick, log)
}

// Drop places ground in the world and starts its cleanup deadline when it is
// not destroy-protected. While Spawn notifies nearby observers, ground
// reports opts.DropperID so those immediate observers receive the animated
// drop packet; observers that discover it later see it simply sitting on
// the ground (see grounditem.Item.DropperID).
func (g *GroundItems) Drop(ground *grounditem.Item, opts DropOptions) {
	if ground == nil {
		return
	}
	ground.SetDropperID(opts.DropperID)
	g.state.Spawn(ground, opts.X, opts.Y, opts.Z, opts.Heading)
	ground.SetDropperID(0)

	if ground.DestroyProtected() {
		return
	}
	g.track(ground, g.destroyDelay(ground, opts.PlayerDropped))
}

// Load restores previously saved ground items and clears their persisted
// countdowns into live cleanup deadlines.
func (g *GroundItems) Load(rows []item.GroundSnapshot, templates *item.Table) error {
	now := g.now()
	for _, row := range rows {
		tmpl, ok := templates.Get(row.TemplateID)
		if !ok {
			return fmt.Errorf("ground items: item template %d not loaded", row.TemplateID)
		}
		row.Instance.ManaLeft = tmpl.InitialManaLeft()
		ground, err := grounditem.New(row.Instance, tmpl)
		if err != nil {
			return err
		}
		g.state.Spawn(ground, row.X, row.Y, row.Z, 0)

		var expiresAt time.Time
		if row.TimeLeftMillis != 0 {
			expiresAt = now.Add(time.Duration(row.TimeLeftMillis) * time.Millisecond)
		}
		g.mu.Lock()
		g.items[ground.ObjectID()] = groundItemEntry{item: ground, expiresAt: expiresAt}
		g.mu.Unlock()
	}
	return nil
}

// Tick despawns all items whose cleanup deadline has passed.
func (g *GroundItems) Tick() {
	now := g.now()
	var expired []world.Tracked

	g.mu.Lock()
	for id, entry := range g.items {
		if entry.expiresAt.IsZero() || now.Before(entry.expiresAt) {
			continue
		}
		delete(g.items, id)
		expired = append(expired, entry.item)
	}
	g.mu.Unlock()

	g.state.DespawnAll(expired)
}

// Remove stops tracking ground, usually after pickup or explicit destruction.
func (g *GroundItems) Remove(ground *grounditem.Item) {
	if ground == nil {
		return
	}
	g.mu.Lock()
	delete(g.items, ground.ObjectID())
	g.mu.Unlock()
}

// Len returns the number of tracked ground items.
func (g *GroundItems) Len() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.items)
}

// Snapshots returns the persisted representation of tracked items. The skip
// callback can filter item ids that should not be saved.
func (g *GroundItems) Snapshots(skip func(itemID int32) bool) []item.GroundSnapshot {
	now := g.now()
	g.mu.RLock()
	defer g.mu.RUnlock()

	out := make([]item.GroundSnapshot, 0, len(g.items))
	for _, entry := range g.items {
		if skip != nil && skip(entry.item.ItemID()) {
			continue
		}
		var left int64
		if !entry.expiresAt.IsZero() {
			left = entry.expiresAt.Sub(now).Milliseconds()
		}
		out = append(out, entry.item.Snapshot(left))
	}
	return out
}

func (g *GroundItems) track(ground *grounditem.Item, delay time.Duration) {
	var expiresAt time.Time
	if delay != 0 {
		expiresAt = g.now().Add(delay)
	}
	g.mu.Lock()
	g.items[ground.ObjectID()] = groundItemEntry{item: ground, expiresAt: expiresAt}
	g.mu.Unlock()
}

func (g *GroundItems) destroyDelay(ground *grounditem.Item, playerDropped bool) time.Duration {
	delay, ok := g.opts.SpecialAutoDestroy[ground.ItemID()]
	if !ok {
		switch {
		case ground.Herb():
			delay = g.opts.HerbAutoDestroy
		case ground.Equipable():
			delay = g.opts.EquipableAutoDestroy
		default:
			delay = g.opts.ItemAutoDestroy
		}
	}
	if playerDropped {
		delay *= time.Duration(g.opts.PlayerDroppedMultiplier)
	}
	return delay
}
