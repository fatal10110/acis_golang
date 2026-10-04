// Package cursedweapon owns the lifecycle of the cursed weapons: which of
// them lies on the ground or is held, by whom, at what stage, how long it
// has left, and its persistence in cursed_weapons.
//
// Only one instance of each weapon exists. A monster kill may drop one
// that is not already out; whoever then obtains it is its holder until it
// drops on the holder's death, or until its time runs out, the holder going
// a day without a player kill or the weapon lying a full hour on the
// ground. Each player kill brings the next stage closer.
//
// The package decides; the caller acts. Every transition is made under one
// lock and reported as a value the caller applies to the players and the
// world: a held weapon's end, the stage a kill reached, the holder its
// remaining time is told to. The database rows are written here, on a
// persistence lane.
package cursedweapon

import (
	"context"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/rs/zerolog"
)

// Timer periods: the hunger and life checks run every minute, a weapon
// left on the ground ends after an hour, and a holder is told its time
// left every 60 hunger checks.
const (
	minute         = time.Minute
	groundLifetime = time.Hour
	reminderEvery  = 60
)

// HolderKarma is the karma a holder carries while it holds a weapon.
const HolderKarma = 9999999

// taskTimeout bounds one database write.
const taskTimeout = 10 * time.Second

// writeLane is the persistence lane every cursed_weapons write is queued
// on, so they land in the order the weapons changed. No character has
// object id 0.
const writeLane int32 = 0

// Row is one stored cursed_weapons row: a held weapon's state.
type Row struct {
	ItemID                int32
	PlayerID              int32
	PlayerKarma           int32
	PlayerPKKills         int32
	NbKills               int32
	CurrentStage          int32
	NumberBeforeNextStage int32
	// HungryTime is the minutes left before the weapon ends for want of a
	// kill.
	HungryTime int32
	// EndTime is when the weapon ends, in Unix milliseconds.
	EndTime int64
}

// Store persists the held weapons and gives a former holder back what it
// had.
type Store interface {
	Load(ctx context.Context) ([]Row, error)
	// Insert stores a newly held weapon's row, replacing any stale one.
	Insert(ctx context.Context, row Row) error
	Update(ctx context.Context, row Row) error
	Delete(ctx context.Context, itemID int32) error
	// ReleaseHolder sets the former holder's karma and PK kills back and,
	// with removeItem, deletes the weapon from its stored items.
	ReleaseHolder(ctx context.Context, playerID, itemID, karma, pkKills int32, removeItem bool) error
}

// Writer runs a database write later, on ownerID's persistence lane.
type Writer interface {
	Enqueue(ownerID int32, job func()) bool
}

// Roll returns a random int in [0, n).
type Roll func(n int) int

// Manager holds every cursed weapon's state. It is safe for concurrent use.
type Manager struct {
	store  Store
	writes Writer
	log    zerolog.Logger
	now    func() time.Time

	// byID and order are fixed at construction.
	byID  map[int32]*weapon
	order []*weapon

	// mu guards every weapon's state; a write is queued while it is held,
	// so the lane order is the order the weapons changed in.
	mu sync.Mutex
}

// weapon is one cursed weapon's definition and state.
type weapon struct {
	def entity.CursedWeapon

	activated, dropped bool

	// playerID is the holder, or the last one while the weapon lies where
	// its holder dropped it.
	playerID                   int32
	playerKarma, playerPK      int32
	nbKills, stage, nextAt     int32
	hungry                     int32
	endTime                    int64
	groundObjectID             int32
	groundAt                   location.Location
	hasGroundAt                bool
	dailyAt, overallAt, dropAt time.Time
	// dailyRuns counts the hunger checks since the holder took the weapon,
	// for the hourly reminder.
	dailyRuns int
}

// New returns the weapons of table, none of them out yet. Restore loads
// the held ones. writes queues the database writes (run inline when nil);
// now is the clock (time.Now when nil).
func New(table *entity.CursedWeaponTable, store Store, writes Writer, log zerolog.Logger, now func() time.Time) *Manager {
	if now == nil {
		now = time.Now
	}
	m := &Manager{store: store, writes: writes, log: log, now: now, byID: map[int32]*weapon{}}
	if table == nil {
		return m
	}
	for _, id := range hashOrder(table.IDs()) {
		def, _ := table.Weapon(id)
		w := &weapon{def: def, stage: 1}
		m.byID[id] = w
		m.order = append(m.order, w)
	}
	return m
}

// hashOrder lists ids in the iteration order of a hash map the reference
// filled with them: buckets of a table of 16 doubled whenever it gets three
// quarters full, a bucket being the low bits of the id folded with its high
// half, and ids sharing a bucket in ascending order.
func hashOrder(ids []int32) []int32 {
	buckets := uint32(16)
	for len(ids) > int(buckets-buckets/4) {
		buckets *= 2
	}
	bucket := func(id int32) uint32 {
		h := uint32(id)
		return (h ^ h>>16) & (buckets - 1)
	}
	out := slices.Clone(ids)
	slices.SortStableFunc(out, func(a, b int32) int {
		if ba, bb := bucket(a), bucket(b); ba != bb {
			return int(ba) - int(bb)
		}
		return int(a) - int(b)
	})
	return out
}

// IsCursed reports whether itemID is a cursed weapon.
func (m *Manager) IsCursed(itemID int32) bool {
	if m == nil {
		return false
	}
	_, ok := m.byID[itemID]
	return ok
}

// Stage returns the stage itemID's weapon is at, 0 for anything else.
func (m *Manager) Stage(itemID int32) int32 {
	if m == nil {
		return 0
	}
	w := m.byID[itemID]
	if w == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return w.stage
}

// Skill returns the skill id itemID's weapon gives its holder.
func (m *Manager) Skill(itemID int32) (int32, bool) {
	if m == nil {
		return 0, false
	}
	w := m.byID[itemID]
	if w == nil {
		return 0, false
	}
	return int32(w.def.Skill.ID), true
}

// TimeLeft returns how long itemID's weapon has left before it ends.
func (m *Manager) TimeLeft(itemID int32) time.Duration {
	if m == nil {
		return 0
	}
	w := m.byID[itemID]
	if w == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return time.Duration(w.endTime-m.now().UnixMilli()) * time.Millisecond
}

// Status is one weapon out in the world.
type Status struct {
	ItemID int32
	// Activated: the weapon is held, by HolderID.
	Activated bool
	HolderID  int32
	// GroundAt is where the weapon last lay on the ground, when
	// HasGroundAt. A weapon taken from the ground keeps it.
	GroundAt    location.Location
	HasGroundAt bool
}

// Active returns every weapon held or lying on the ground.
func (m *Manager) Active() []Status {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Status
	for _, w := range m.order {
		if !w.active() {
			continue
		}
		out = append(out, Status{
			ItemID:      w.def.ItemID,
			Activated:   w.activated,
			HolderID:    w.playerID,
			GroundAt:    w.groundAt,
			HasGroundAt: w.dropped && w.hasGroundAt,
		})
	}
	return out
}

func (w *weapon) active() bool { return w.activated || w.dropped }

// RollDrop rolls a monster kill's cursed weapon: each weapon not out yet
// rolls its drop rate in a million, in turn, until one drops. The dropped
// weapon starts its life and its hour on the ground; the caller lays it
// down and reports where with PlaceOnGround.
func (m *Manager) RollDrop(roll Roll) (int32, bool) {
	if m == nil {
		return 0, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	for _, w := range m.order {
		if w.active() {
			continue
		}
		if roll(1000000) >= w.def.DropRate {
			continue
		}
		w.activated, w.dropped = false, true
		w.endTime = now.Add(time.Duration(w.def.Duration) * time.Hour).UnixMilli()
		w.overallAt = now.Add(minute)
		w.dropAt = now.Add(groundLifetime)
		return w.def.ItemID, true
	}
	return 0, false
}

// PlaceOnGround records where itemID's weapon lies: the ground item
// objectID, at.
func (m *Manager) PlaceOnGround(itemID, objectID int32, at location.Location) {
	w := m.weapon(itemID)
	if w == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	w.groundObjectID, w.groundAt, w.hasGroundAt = objectID, at, true
}

func (m *Manager) weapon(itemID int32) *weapon {
	if m == nil {
		return nil
	}
	return m.byID[itemID]
}

// Holder is the player obtaining a weapon.
type Holder struct {
	ObjectID int32
	Karma    int32
	PKKills  int32
	// HeldItemID is the weapon the player already holds, 0 for none.
	HeldItemID int32
}

// Activation is what obtaining a weapon did.
type Activation struct {
	// Assimilated: the player already held HeldItemID, which took the new
	// weapon in; the new one ended (its end is End, which still has a
	// holder to release when someone else held it) and the held one went
	// up a stage when RankedUp.
	Assimilated bool
	HeldItemID  int32
	RankedUp    bool
	End         EndOfLife
	// Stage is the stage of the weapon the player now holds.
	Stage int32
}

// Activate makes h the holder of itemID's weapon, which it just obtained.
// One already holding a weapon cannot take a second: the held one goes up
// a stage and the new one ends. ok is false for an item that is no cursed
// weapon.
func (m *Manager) Activate(itemID int32, h Holder, roll Roll) (Activation, bool) {
	w := m.weapon(itemID)
	if w == nil {
		return Activation{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if held := m.byID[h.HeldItemID]; held != nil && h.HeldItemID != 0 {
		ranked := held.rankUp()
		// The new weapon ends in the player's inventory, which the caller
		// takes it out of: it no longer lies on the ground.
		if !w.activated {
			w.groundObjectID = 0
		}
		end := m.endOfLifeLocked(w)
		return Activation{Assimilated: true, HeldItemID: h.HeldItemID, RankedUp: ranked, End: end, Stage: held.stage}, true
	}
	now := m.now()
	w.activated = true
	w.playerID, w.playerKarma, w.playerPK = h.ObjectID, h.Karma, h.PKKills
	w.nextAt = w.rollStageKills(roll)
	w.hungry = int32(w.def.DurationLost * 60)
	w.dailyAt, w.dailyRuns = now.Add(minute), 0
	w.dropAt = time.Time{}
	row := w.row()
	m.writeLocked("insert", func(ctx context.Context, st Store) error { return st.Insert(ctx, row) })
	return Activation{Stage: w.stage}, true
}

// rollStageKills rolls how many kills the next stage takes: between half
// and one and a half times the weapon's stage kills, both included.
func (w *weapon) rollStageKills(roll Roll) int32 {
	lo := int(math.Round(float64(w.def.StageKills) * 0.5))
	hi := int(math.Round(float64(w.def.StageKills) * 1.5))
	return int32(lo + roll(hi-lo+1))
}

// rankUp takes w a stage up, short of its skill's top level.
func (w *weapon) rankUp() bool {
	if w.stage >= int32(w.def.Skill.Level) {
		return false
	}
	w.stage++
	return true
}

// Death is what a holder's death did to its weapon.
type Death struct {
	// Disappeared: the weapon ended (End). Otherwise it dropped: the holder
	// gets Karma and PKKills back, and the caller lays the weapon down and
	// reports where with PlaceOnGround.
	Disappeared bool
	End         EndOfLife
	Karma       int32
	PKKills     int32
}

// HolderDied settles itemID's weapon when its holder dies: it ends when
// the roll in a hundred is at most its disappear chance, and drops with
// its stage back to 1 otherwise. ok is false when the weapon is not held.
func (m *Manager) HolderDied(itemID int32, roll Roll) (Death, bool) {
	w := m.weapon(itemID)
	if w == nil {
		return Death{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !w.activated {
		return Death{}, false
	}
	if roll(100) <= w.def.DisappearChance {
		return Death{Disappeared: true, End: m.endOfLifeLocked(w)}, true
	}
	w.activated, w.dropped = false, true
	w.dailyAt = time.Time{}
	w.dropAt = m.now().Add(groundLifetime)
	w.stage = 1
	m.deleteRowLocked(w.def.ItemID)
	return Death{Karma: w.playerKarma, PKKills: w.playerPK}, true
}

// Kill counts a player kill by the holder playerID of itemID's weapon: its
// hunger is fed, and once enough kills are in the weapon goes up a stage
// and the count starts over. ok is false when playerID does not hold it.
func (m *Manager) Kill(itemID, playerID int32, roll Roll) (rankedUp bool, stage int32, ok bool) {
	w := m.weapon(itemID)
	if w == nil {
		return false, 0, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !w.activated || w.playerID != playerID {
		return false, 0, false
	}
	w.nbKills++
	w.hungry = int32(w.def.DurationLost * 60)
	if w.nbKills >= w.nextAt {
		w.nbKills = 0
		w.nextAt = w.rollStageKills(roll)
		rankedUp = w.rankUp()
	}
	return rankedUp, w.stage, true
}

// Held returns the weapon playerID holds and its stage, for a holder
// coming back into the world.
func (m *Manager) Held(playerID int32) (itemID, stage int32, ok bool) {
	if m == nil {
		return 0, 0, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, w := range m.order {
		if w.activated && w.playerID == playerID {
			return w.def.ItemID, w.stage, true
		}
	}
	return 0, 0, false
}

// EndOfLife is a weapon's end, for the caller to apply.
type EndOfLife struct {
	ItemID int32
	// Held: the weapon was held by HolderID, who gets Karma and PKKills
	// back and loses the weapon.
	Held     bool
	HolderID int32
	Karma    int32
	PKKills  int32
	// GroundObjectID is the ground item to take out of the world, when
	// the weapon was not held; 0 for none.
	GroundObjectID int32
}

// Expire ends itemID's weapon wherever it is. ok is false when it is not
// out.
func (m *Manager) Expire(itemID int32) (EndOfLife, bool) {
	w := m.weapon(itemID)
	if w == nil {
		return EndOfLife{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !w.active() {
		return EndOfLife{}, false
	}
	return m.endOfLifeLocked(w), true
}

// endOfLifeLocked ends w: its timers stop, its row goes and its state
// starts over. The caller holds mu.
func (m *Manager) endOfLifeLocked(w *weapon) EndOfLife {
	end := EndOfLife{ItemID: w.def.ItemID}
	if w.activated {
		end.Held = true
		end.HolderID, end.Karma, end.PKKills = w.playerID, w.playerKarma, w.playerPK
	} else {
		end.GroundObjectID = w.groundObjectID
	}
	m.deleteRowLocked(w.def.ItemID)
	*w = weapon{def: w.def, stage: 1}
	return end
}

// Reminder is the time a holder is told it has left.
type Reminder struct {
	ItemID   int32
	HolderID int32
	Left     time.Duration
}

// Tick runs every timer due by now: the hunger check, which ends a weapon
// whose holder went too long without a kill and has the holder told its
// time left every hour; the life check, which ends a weapon past its end
// time and stores the state of the others; and the end of a weapon left on
// the ground for an hour.
func (m *Manager) Tick(now time.Time) ([]EndOfLife, []Reminder) {
	if m == nil {
		return nil, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var ends []EndOfLife
	var reminders []Reminder
	for _, w := range m.order {
		end, reminder, ended := m.tickLocked(w, now)
		reminders = append(reminders, reminder...)
		if ended {
			ends = append(ends, end)
		}
	}
	return ends, reminders
}

// tickLocked runs w's timers due by now, in the order they fall due.
func (m *Manager) tickLocked(w *weapon, now time.Time) (EndOfLife, []Reminder, bool) {
	var reminders []Reminder
	for {
		next, kind := w.nextTimer()
		if next.IsZero() || now.Before(next) {
			return EndOfLife{}, reminders, false
		}
		switch kind {
		case timerDaily:
			w.dailyAt = w.dailyAt.Add(minute)
			w.hungry--
			w.dailyRuns++
			if w.hungry <= 0 {
				return m.endOfLifeLocked(w), reminders, true
			}
			if w.dailyRuns%reminderEvery == 0 {
				left := time.Duration(w.endTime-now.UnixMilli()) * time.Millisecond
				reminders = append(reminders, Reminder{ItemID: w.def.ItemID, HolderID: w.playerID, Left: left})
			}
		case timerOverall:
			w.overallAt = w.overallAt.Add(minute)
			if now.UnixMilli() >= w.endTime {
				return m.endOfLifeLocked(w), reminders, true
			}
			if w.activated {
				row := w.row()
				m.writeLocked("update", func(ctx context.Context, st Store) error { return st.Update(ctx, row) })
			}
		case timerDrop:
			w.dropAt = time.Time{}
			if w.dropped && !w.activated {
				return m.endOfLifeLocked(w), reminders, true
			}
		}
	}
}

type timerKind int

const (
	timerDaily timerKind = iota
	timerOverall
	timerDrop
)

// nextTimer returns w's earliest armed timer, zero when none is.
func (w *weapon) nextTimer() (time.Time, timerKind) {
	var at time.Time
	var kind timerKind
	for k, t := range [...]time.Time{timerDaily: w.dailyAt, timerOverall: w.overallAt, timerDrop: w.dropAt} {
		if t.IsZero() {
			continue
		}
		if at.IsZero() || t.Before(at) {
			at, kind = t, timerKind(k)
		}
	}
	return at, kind
}

// Restore loads the held weapons. A weapon whose time ran out while the
// server was down ends at once, its holder getting its karma and PK kills
// back and losing the weapon from its stored items.
func (m *Manager) Restore(ctx context.Context) error {
	if m == nil || m.store == nil {
		return nil
	}
	rows, err := m.store.Load(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	held := 0
	for _, row := range rows {
		w := m.byID[row.ItemID]
		if w == nil {
			continue
		}
		w.activated = true
		w.playerID, w.playerKarma, w.playerPK = row.PlayerID, row.PlayerKarma, row.PlayerPKKills
		w.nbKills, w.stage, w.nextAt = row.NbKills, row.CurrentStage, row.NumberBeforeNextStage
		w.hungry, w.endTime = row.HungryTime, row.EndTime
		if w.endTime-now.UnixMilli() <= 0 {
			m.releaseHolderLocked(m.endOfLifeLocked(w), true)
			continue
		}
		w.dailyAt, w.overallAt = now.Add(minute), now.Add(minute)
		held++
	}
	m.log.Info().Int("weapons", len(m.order)).Int("held", held).Msg("cursed weapons: restored")
	return nil
}

// ReleaseHolder stores what an ended weapon gives its former holder back:
// its karma and PK kills and, with removeItem, the weapon taken out of its
// stored items, for a holder who is not online to lose it from its
// inventory.
func (m *Manager) ReleaseHolder(end EndOfLife, removeItem bool) {
	if m == nil || !end.Held {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.releaseHolderLocked(end, removeItem)
}

func (m *Manager) releaseHolderLocked(end EndOfLife, removeItem bool) {
	m.writeOnLocked(end.HolderID, "release holder", func(ctx context.Context, st Store) error {
		return st.ReleaseHolder(ctx, end.HolderID, end.ItemID, end.Karma, end.PKKills, removeItem)
	})
}

func (w *weapon) row() Row {
	return Row{
		ItemID:                w.def.ItemID,
		PlayerID:              w.playerID,
		PlayerKarma:           w.playerKarma,
		PlayerPKKills:         w.playerPK,
		NbKills:               w.nbKills,
		CurrentStage:          w.stage,
		NumberBeforeNextStage: w.nextAt,
		HungryTime:            w.hungry,
		EndTime:               w.endTime,
	}
}

func (m *Manager) deleteRowLocked(itemID int32) {
	m.writeLocked("delete", func(ctx context.Context, st Store) error { return st.Delete(ctx, itemID) })
}

// writeLocked queues fn on the cursed weapons' lane while mu is held.
func (m *Manager) writeLocked(what string, fn func(context.Context, Store) error) {
	m.writeOnLocked(writeLane, what, fn)
}

// writeOnLocked queues fn on lane while mu is held, or runs it at once
// without a writer. Each write gets taskTimeout.
func (m *Manager) writeOnLocked(lane int32, what string, fn func(context.Context, Store) error) {
	if m.store == nil {
		return
	}
	store, log := m.store, m.log
	job := func() {
		ctx, cancel := context.WithTimeout(context.Background(), taskTimeout)
		defer cancel()
		if err := fn(ctx, store); err != nil {
			log.Error().Err(err).Msg("cursed weapons: " + what)
		}
	}
	if m.writes == nil {
		job()
		return
	}
	if !m.writes.Enqueue(lane, job) {
		log.Error().Msg("cursed weapons: " + what + ": persistence closed")
	}
}
