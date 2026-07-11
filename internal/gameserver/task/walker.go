package task

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
)

const (
	// WalkerTick is the fixed route-walker delay check interval.
	WalkerTick = time.Second

	walkerGeoFailLimit = 10
)

// WalkerActor is the narrow NPC surface route walking needs.
type WalkerActor interface {
	ObjectID() int32
	Position() location.Location
	Moving() bool
	MoveToLocation(location.Location) (move.Event, error)
	TeleportTo(location.Location)
	GeoPathFailCount() int
	ResetGeoPathFailCount()
	AddGeoPathFailCount()
	SayNPCString(id int)
	SocialAction(id int)
}

// WalkerPath answers route reachability before a walking NPC requests movement.
type WalkerPath interface {
	CanMove(origin, target location.Location) bool
	HasPath(origin, target location.Location) bool
}

// PathFinder is the geodata path search method WalkerPath can wrap.
type PathFinder interface {
	Find(origin, target location.Location) ([]location.Location, int, bool)
}

// MoveGeo is the straight-line movement query WalkerPath can wrap.
type MoveGeo interface {
	CanMove(ox, oy, oz, tx, ty, tz int) bool
}

// GeoPath adapts the geodata engine and pathfinder to WalkerPath.
type GeoPath struct {
	Geo    MoveGeo
	Finder PathFinder
}

// CanMove reports whether origin can directly reach target.
func (p GeoPath) CanMove(origin, target location.Location) bool {
	return p.Geo != nil && p.Geo.CanMove(origin.X, origin.Y, origin.Z, target.X, target.Y, target.Z)
}

// HasPath reports whether pathfinding found a non-empty route to target.
func (p GeoPath) HasPath(origin, target location.Location) bool {
	if p.Finder == nil {
		return false
	}
	path, _, ok := p.Finder.Find(origin, target)
	return ok && len(path) > 0
}

// Walker advances registered NPC route walkers and releases delayed walkers
// on the one-second task tick. All methods are safe for concurrent use; mu
// guards entries and their per-actor route state.
type Walker struct {
	routes route.WalkerRoutes
	path   WalkerPath
	now    func() time.Time

	mu      sync.Mutex
	entries map[int32]*walkerEntry
}

type walkerEntry struct {
	actor    WalkerActor
	route    string
	npc      string
	index    int
	onRoute  bool
	reverse  bool
	wakeTime time.Time
}

// NewWalker returns a route walker over loaded walkerRoutes.xml data.
func NewWalker(routes route.WalkerRoutes, path WalkerPath, now func() time.Time) (*Walker, error) {
	if path == nil {
		return nil, errors.New("task: walker path is nil")
	}
	if now == nil {
		now = time.Now
	}
	return &Walker{
		routes:  routes,
		path:    path,
		now:     now,
		entries: make(map[int32]*walkerEntry),
	}, nil
}

// Start launches the fixed one-second walker task.
func (w *Walker) Start(log zerolog.Logger) *scheduler.Ticker {
	return scheduler.Start(WalkerTick, func() {
		for _, err := range w.Tick() {
			log.Error().Err(err).Msg("walker route tick")
		}
	}, log)
}

// StartRoute registers actor on routeName/npcName and immediately requests
// movement toward the nearest route node.
func (w *Walker) StartRoute(actor WalkerActor, routeName, npcName string) error {
	if actor == nil {
		return errors.New("task: nil walker actor")
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if _, err := w.nodes(routeName, npcName); err != nil {
		return err
	}
	entry := &walkerEntry{actor: actor, route: routeName, npc: npcName}
	w.entries[actor.ObjectID()] = entry
	return w.moveToNextPoint(entry)
}

// StopRoute removes actor from route walking.
func (w *Walker) StopRoute(actor WalkerActor) {
	if actor == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.entries, actor.ObjectID())
}

// Arrived handles actor reaching its current route node.
func (w *Walker) Arrived(actor WalkerActor) error {
	if actor == nil {
		return errors.New("task: nil walker actor")
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	entry, ok := w.entries[actor.ObjectID()]
	if !ok {
		return nil
	}
	nodes, err := w.nodes(entry.route, entry.npc)
	if err != nil {
		entry.onRoute = false
		return err
	}
	if !entry.onRoute {
		return nil
	}
	if entry.index < 0 || entry.index >= len(nodes) {
		entry.onRoute = false
		return fmt.Errorf("task: walker %d route %q index %d out of range", actor.ObjectID(), entry.route, entry.index)
	}

	node := nodes[entry.index]
	if node.NPCStringID != 0 {
		actor.SayNPCString(node.NPCStringID)
	}
	if node.DelayMillis > 0 {
		if node.SocialID > 0 {
			actor.SocialAction(node.SocialID)
		}
		entry.wakeTime = w.now().Add(time.Duration(node.DelayMillis) * time.Millisecond)
		return nil
	}
	return w.moveToNextPoint(entry)
}

// MoveToNextPoint immediately requests actor's next route node.
func (w *Walker) MoveToNextPoint(actor WalkerActor) error {
	if actor == nil {
		return errors.New("task: nil walker actor")
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	entry, ok := w.entries[actor.ObjectID()]
	if !ok {
		return fmt.Errorf("task: walker %d is not registered", actor.ObjectID())
	}
	entry.wakeTime = time.Time{}
	return w.moveToNextPoint(entry)
}

// Tick releases delayed route walkers whose wait has elapsed and that are no
// longer moving. It returns per-entry errors so tests and callers can surface
// bad route state without stopping the recurring task.
func (w *Walker) Tick() []error {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := w.now()
	var errs []error
	for id, entry := range w.entries {
		if entry.wakeTime.IsZero() || now.Before(entry.wakeTime) {
			continue
		}
		if entry.actor.Moving() {
			continue
		}
		entry.wakeTime = time.Time{}
		if err := w.moveToNextPoint(entry); err != nil {
			errs = append(errs, fmt.Errorf("task: walker %d: %w", id, err))
		}
	}
	return errs
}

func (w *Walker) moveToNextPoint(entry *walkerEntry) error {
	nodes, err := w.nodes(entry.route, entry.npc)
	if err != nil {
		entry.onRoute = false
		return err
	}

	previousIndex, previousOnRoute, previousReverse := entry.index, entry.onRoute, entry.reverse
	if !entry.onRoute {
		entry.index = nearestNode(entry.actor.Position(), nodes)
	} else {
		if entry.actor.GeoPathFailCount() >= walkerGeoFailLimit {
			entry.index = 0
			entry.actor.TeleportTo(nodes[0].Location)
			entry.reverse = false
			entry.actor.ResetGeoPathFailCount()
		}

		switch {
		case entry.reverse && entry.index > 0:
			entry.index--
			if entry.index == 0 {
				entry.reverse = false
			}
		case entry.index < len(nodes)-1:
			entry.index++
		default:
			entry.index = 0
		}
	}

	node := nodes[entry.index]
	if !w.path.CanMove(entry.actor.Position(), node.Location) && !w.path.HasPath(entry.actor.Position(), node.Location) {
		entry.actor.AddGeoPathFailCount()
		switch {
		case entry.index == 0 && len(nodes) > 1:
			entry.index = len(nodes) - 2
			entry.reverse = true
		case entry.index > 0:
			entry.index--
		}
		node = nodes[entry.index]
	}

	if _, err := entry.actor.MoveToLocation(node.Location); err != nil {
		entry.index = previousIndex
		entry.onRoute = previousOnRoute
		entry.reverse = previousReverse
		entry.wakeTime = w.now().Add(WalkerTick)
		return err
	}
	entry.onRoute = true
	return nil
}

func (w *Walker) nodes(routeName, npcName string) ([]route.WalkerLocation, error) {
	byNPC, ok := w.routes[routeName]
	if !ok {
		return nil, fmt.Errorf("task: walker route %q not found", routeName)
	}
	nodes, ok := byNPC[npcName]
	if !ok {
		return nil, fmt.Errorf("task: walker route %q npc %q not found", routeName, npcName)
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("task: walker route %q npc %q has no nodes", routeName, npcName)
	}
	return nodes, nil
}

func nearestNode(origin location.Location, nodes []route.WalkerLocation) int {
	bestIndex := 0
	bestDistance := math.Inf(1)
	for i, node := range nodes {
		d := distance3D(origin, node.Location)
		if d < bestDistance {
			bestDistance = d
			bestIndex = i
		}
	}
	return bestIndex
}

func distance3D(a, b location.Location) float64 {
	dx := float64(a.X - b.X)
	dy := float64(a.Y - b.Y)
	dz := float64(a.Z - b.Z)
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}
