package task

import (
	"errors"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// ShadowItemTick is the fixed shadow-item mana decay interval.
const ShadowItemTick = time.Second

// ShadowItemEffects delivers a tracked shadow item's client-visible side
// effects and the removal actions that belong to the actor's own
// inventory/world state, not this package.
type ShadowItemEffects interface {
	// ManaThreshold notifies actorID that inst's remaining mana just
	// crossed a notable threshold (1, 5, or 10 minutes left). Called
	// exactly once per crossing, not on every tick.
	ManaThreshold(actorID int32, inst *item.Instance, secondsLeft int)
	// Expire unequips (if still equipped) and destroys inst: its mana ran
	// out. Called once, after which this manager stops tracking inst.
	Expire(actorID int32, inst *item.Instance)
}

type shadowItemEntry struct {
	actorID int32
	inst    *item.Instance
}

// ShadowItems tracks equipped shadow items — time-limited weapons or armor
// — and decays each one's remaining mana by one second per tick while it
// stays equipped, firing a threshold notification at 10/5/1 minutes left
// and an expiry event once mana reaches zero.
//
// The reference manager also refreshes a shadow item's persisted state
// once a minute while equipped; that persistence trigger's own guard
// condition (remaining mana modulo 60 equals 60) can never be true for any
// non-negative mana value, so it never actually fires there either — this
// port simply doesn't reproduce that dead branch.
//
// mu guards entries. Mutable item fields are guarded by item.Instance.
type ShadowItems struct {
	effects ShadowItemEffects

	mu      sync.Mutex
	entries map[int32]shadowItemEntry // keyed by item object id
}

// NewShadowItems returns an empty shadow-item tracker that reports through
// effects.
func NewShadowItems(effects ShadowItemEffects) (*ShadowItems, error) {
	if effects == nil {
		return nil, errors.New("task: shadow item effects is nil")
	}
	return &ShadowItems{effects: effects, entries: make(map[int32]shadowItemEntry)}, nil
}

// Start launches the fixed one-second shadow-item decay task.
func (s *ShadowItems) Start(log zerolog.Logger) *scheduler.Ticker {
	return scheduler.Start(ShadowItemTick, s.Tick, log)
}

// Track starts decaying inst's mana while it stays equipped by actorID.
// tmpl must be inst's own template; a non-shadow item is ignored. Every
// equip after the very first one (mana still at tmpl's full duration)
// costs an extra minute of mana, discouraging repeated equip/unequip
// cycles to stall the decay.
func (s *ShadowItems) Track(actorID int32, inst *item.Instance, tmpl *item.Template) {
	if inst == nil || tmpl == nil || !inst.ShadowItem(tmpl) {
		return
	}
	if inst.Snapshot().ManaLeft != tmpl.InitialManaLeft() {
		inst.DecreaseMana(60)
	}

	s.mu.Lock()
	s.entries[inst.ObjectID] = shadowItemEntry{actorID: actorID, inst: inst}
	s.mu.Unlock()
}

// Untrack stops decaying inst's mana: it was unequipped.
func (s *ShadowItems) Untrack(inst *item.Instance) {
	if inst == nil {
		return
	}
	s.mu.Lock()
	delete(s.entries, inst.ObjectID)
	s.mu.Unlock()
}

// Remove stops tracking every item currently tracked for actorID (e.g. the
// player disconnected).
func (s *ShadowItems) Remove(actorID int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, e := range s.entries {
		if e.actorID == actorID {
			delete(s.entries, id)
		}
	}
}

// Tracked reports whether inst is currently tracked.
func (s *ShadowItems) Tracked(inst *item.Instance) bool {
	if inst == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.entries[inst.ObjectID]
	return ok
}

func manaThreshold(secondsLeft int) bool {
	switch secondsLeft {
	case 600, 300, 60:
		return true
	default:
		return false
	}
}

// Tick decreases every tracked item's mana by one second, firing a
// threshold notification at 10/5/1 minutes remaining and an expiry event
// (after which tracking stops for that item) once mana reaches zero.
func (s *ShadowItems) Tick() {
	s.mu.Lock()
	entries := make([]shadowItemEntry, 0, len(s.entries))
	for _, e := range s.entries {
		entries = append(entries, e)
	}
	s.mu.Unlock()

	for _, e := range entries {
		manaLeft := e.inst.DecreaseMana(1)

		if manaLeft <= 0 {
			s.Untrack(e.inst)
			s.effects.Expire(e.actorID, e.inst)
			continue
		}

		if manaThreshold(manaLeft) {
			s.effects.ManaThreshold(e.actorID, e.inst, manaLeft)
		}
	}
}
