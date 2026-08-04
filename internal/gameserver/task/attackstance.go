package task

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
)

// AttackStanceTick is the fixed combat-stance expiry interval.
const AttackStanceTick = time.Second

// AttackStancePeriod is how long combat stance remains active after the
// latest attack action.
const AttackStancePeriod = 15 * time.Second

// LifeCubicID is the healing cubic that does not perform an attack action
// when combat stance is refreshed.
const LifeCubicID = 3

// AttackStanceActor is the narrow actor surface tracked by combat stance.
type AttackStanceActor interface {
	ObjectID() int32
}

// AttackStanceEffects delivers combat-stance timeout side effects.
type AttackStanceEffects interface {
	AutoAttackStop(actor AttackStanceActor)
}

// AttackStanceCubic is the narrow cubic surface refreshed by combat stance.
type AttackStanceCubic interface {
	ID() int
	Action()
}

type attackStanceOwner interface {
	Owner() AttackStanceActor
}

type attackStanceSummoner interface {
	Summon() AttackStanceActor
}

type attackStanceCubics interface {
	Cubics() []AttackStanceCubic
}

type attackStanceEntry struct {
	actor    AttackStanceActor
	deadline time.Time
}

// AttackStance tracks actors whose combat animation should remain active
// until the inactivity period expires.
//
// Add, Remove, and InAttackStance are safe to call concurrently with Tick. mu
// guards entries and the scratch refill. Tick only ever runs on the scheduler
// ticker's single goroutine, one call at a time; ticking enforces that
// contract by panicking on reentrant or concurrent Tick calls.
type AttackStance struct {
	effects AttackStanceEffects
	now     func() time.Time

	mu      sync.Mutex
	entries map[int32]attackStanceEntry
	scratch []attackStanceEntry

	ticking atomic.Bool
}

// NewAttackStance returns an empty combat-stance tracker.
func NewAttackStance(effects AttackStanceEffects, now func() time.Time) (*AttackStance, error) {
	if effects == nil {
		return nil, errors.New("task: attack stance effects is nil")
	}
	if now == nil {
		now = time.Now
	}
	return &AttackStance{effects: effects, now: now, entries: make(map[int32]attackStanceEntry)}, nil
}

// Start launches the fixed one-second combat-stance task.
func (a *AttackStance) Start(log zerolog.Logger) *scheduler.Ticker {
	return scheduler.Start(AttackStanceTick, a.Tick, log)
}

// Add refreshes actor's combat stance timeout.
func (a *AttackStance) Add(actor AttackStanceActor) {
	if actor == nil {
		return
	}
	if c, ok := actor.(attackStanceCubics); ok {
		for _, cubic := range c.Cubics() {
			if cubic != nil && cubic.ID() != LifeCubicID {
				cubic.Action()
			}
		}
	}

	a.mu.Lock()
	a.entries[actor.ObjectID()] = attackStanceEntry{actor: actor, deadline: a.now().Add(AttackStancePeriod)}
	a.mu.Unlock()
}

// Remove stops tracking actor and reports whether it had been tracked.
func (a *AttackStance) Remove(actor AttackStanceActor) bool {
	actor = stanceOwner(actor)
	if actor == nil {
		return false
	}

	a.mu.Lock()
	_, tracked := a.entries[actor.ObjectID()]
	if tracked {
		delete(a.entries, actor.ObjectID())
	}
	a.mu.Unlock()
	return tracked
}

// InAttackStance reports whether actor is currently tracked.
func (a *AttackStance) InAttackStance(actor AttackStanceActor) bool {
	actor = stanceOwner(actor)
	if actor == nil {
		return false
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.entries[actor.ObjectID()]
	return ok
}

// Tick stops combat stance for actors whose inactivity period has elapsed.
func (a *AttackStance) Tick() {
	if !a.ticking.CompareAndSwap(false, true) {
		panic("task: AttackStance.Tick called concurrently; Tick is single-goroutine only")
	}
	defer a.ticking.Store(false)

	now := a.now()

	a.mu.Lock()
	a.scratch = a.scratch[:0]
	for id, entry := range a.entries {
		if now.Before(entry.deadline) {
			continue
		}
		a.scratch = append(a.scratch, entry)
		delete(a.entries, id)
	}
	due := a.scratch
	a.mu.Unlock()

	defer clear(a.scratch)

	for _, entry := range due {
		a.effects.AutoAttackStop(entry.actor)
		if s, ok := entry.actor.(attackStanceSummoner); ok {
			if summon := s.Summon(); summon != nil {
				a.effects.AutoAttackStop(summon)
			}
		}
	}
}

func stanceOwner(actor AttackStanceActor) AttackStanceActor {
	if actor == nil {
		return nil
	}
	if s, ok := actor.(attackStanceOwner); ok {
		if owner := s.Owner(); owner != nil {
			return owner
		}
	}
	return actor
}
