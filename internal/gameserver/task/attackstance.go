package task

import (
	"errors"
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

// AttackStance tracks actors whose combat animation should remain active
// until the inactivity period expires.
//
// Add, Remove, and InAttackStance are safe to call concurrently with Tick.
// Tick only ever runs on the scheduler ticker's single goroutine, one call
// at a time; ticking enforces that contract by logging and returning
// ErrReentrantTick on reentrant or concurrent Tick calls instead of
// running, since Start's caller is the only reliable place that can act on
// the returned error.
type AttackStance struct {
	effects AttackStanceEffects
	now     func() time.Time
	log     zerolog.Logger

	*serialDeadlineRegistry[int32, AttackStanceActor]
}

// NewAttackStance returns an empty combat-stance tracker.
func NewAttackStance(effects AttackStanceEffects, now func() time.Time) (*AttackStance, error) {
	if effects == nil {
		return nil, errors.New("task: attack stance effects is nil")
	}
	if now == nil {
		now = time.Now
	}
	return &AttackStance{effects: effects, now: now, serialDeadlineRegistry: newSerialDeadlineRegistry[int32, AttackStanceActor]()}, nil
}

// Start launches the fixed one-second combat-stance task.
func (a *AttackStance) Start(log zerolog.Logger) *scheduler.Ticker {
	a.log = log
	return scheduler.Start(AttackStanceTick, func() { a.Tick() }, log)
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

	a.add(actor.ObjectID(), actor, a.now().Add(AttackStancePeriod))
}

// Remove stops tracking actor and reports whether it had been tracked.
func (a *AttackStance) Remove(actor AttackStanceActor) bool {
	actor = stanceOwner(actor)
	if actor == nil {
		return false
	}
	return a.remove(actor.ObjectID())
}

// InAttackStance reports whether actor is currently tracked.
func (a *AttackStance) InAttackStance(actor AttackStanceActor) bool {
	actor = stanceOwner(actor)
	if actor == nil {
		return false
	}
	return a.tracked(actor.ObjectID())
}

// Tick stops combat stance for actors whose inactivity period has elapsed.
// It logs and returns ErrReentrantTick without doing anything else if another Tick call is
// already in flight.
func (a *AttackStance) Tick() error {
	if !a.beginTick(a.log, "task: AttackStance.Tick") {
		return ErrReentrantTick
	}
	defer a.endTick()

	a.tickDue(a.now(), func(actor AttackStanceActor) {
		a.effects.AutoAttackStop(actor)
		if s, ok := actor.(attackStanceSummoner); ok {
			if summon := s.Summon(); summon != nil {
				a.effects.AutoAttackStop(summon)
			}
		}
	})
	return nil
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
