package task

import (
	"errors"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// DecayTick is the fixed corpse-decay sweep interval.
const DecayTick = time.Second

// DecayActor is the narrow actor surface tracked by the corpse decay task.
type DecayActor interface {
	ObjectID() int32
	Queued
}

// SummonDecayActor is a corpse-decay entry whose owner linkage is rechecked
// each tick. When OwnerStillLinked reports false, the entry is removed without
// invoking decay effects — matching the reference decay task's orphaned-summon
// cancellation before its deadline check.
type SummonDecayActor interface {
	DecayActor
	OwnerStillLinked() bool
}

// DecayEffects delivers corpse-removal side effects when a tracked actor's
// decay deadline elapses.
type DecayEffects interface {
	Decay(actor DecayActor)
}

// Decay tracks dead actors awaiting corpse removal and fires the removal
// side effect once each actor's display interval elapses.
//
// Add, Cancel, Tracked, and Deadline are safe to call concurrently with Tick.
// Tick only ever runs on the scheduler ticker's single goroutine, one call
// at a time; ticking enforces that contract by logging and returning
// ErrReentrantTick on reentrant or concurrent Tick calls instead of
// running, since Start's caller is the only reliable place that can act on
// the returned error.
type Decay struct {
	effects DecayEffects
	now     func() time.Time
	log     zerolog.Logger

	*serialDeadlineRegistry[int32, DecayActor]
}

// NewDecay returns an empty corpse-decay tracker.
func NewDecay(effects DecayEffects, now func() time.Time) (*Decay, error) {
	if effects == nil {
		return nil, errors.New("task: decay effects is nil")
	}
	if now == nil {
		now = time.Now
	}
	return &Decay{effects: effects, now: now, serialDeadlineRegistry: newSerialDeadlineRegistry[int32, DecayActor]()}, nil
}

// Start launches the fixed one-second corpse-decay task.
func (d *Decay) Start(log zerolog.Logger) *scheduler.Ticker {
	d.log = log
	// Tick already logs its only error, ErrReentrantTick.
	return scheduler.Start(DecayTick, func() { _ = d.Tick() }, log)
}

// Add schedules actor's corpse for removal after interval elapses,
// replacing any deadline already tracked for it. It returns the stored
// deadline so callers that expose corpse-targeting state can share the
// same cutoff the task will use.
func (d *Decay) Add(actor DecayActor, interval time.Duration) time.Time {
	if actor == nil {
		return time.Time{}
	}
	deadline := d.now().Add(interval)
	d.add(actor.ObjectID(), actor, deadline)
	return deadline
}

// Cancel stops tracking actor and reports whether it had been tracked.
func (d *Decay) Cancel(actor DecayActor) bool {
	if actor == nil {
		return false
	}
	return d.remove(actor.ObjectID())
}

// Tracked reports whether actor currently has a pending decay deadline.
func (d *Decay) Tracked(actor DecayActor) bool {
	if actor == nil {
		return false
	}
	return d.tracked(actor.ObjectID())
}

// Deadline returns actor's pending decay deadline, if one is tracked.
func (d *Decay) Deadline(actor DecayActor) (time.Time, bool) {
	if actor == nil {
		return time.Time{}, false
	}
	return d.deadlineOf(actor.ObjectID())
}

// Tick removes orphaned summon entries, then removes and decays every actor
// whose deadline has passed. It logs and returns ErrReentrantTick without
// doing anything else if another Tick call is already in flight.
func (d *Decay) Tick() error {
	if !d.beginTick(d.log, "task: Decay.Tick") {
		return ErrReentrantTick
	}
	defer d.endTick()

	d.cancelUnlinkedSummons()
	d.tickDue(d.now(), d.post)
	return nil
}

// post runs actor's decay on actor's queue. A corpse whose owner leaves
// moves to a queue of its own before the owner's queue closes, and a pet's
// corpse moves back onto its owner's queue when the owner returns for it, so
// a post the queue read first refuses goes to the one actor has moved to, and
// a decay a queue accepted just before actor moved off it is handed on rather
// than run beside actor's work on the new queue. Dropping it would leave the
// corpse in the world for good. An actor moves only while the queue it leaves
// is owned, so the check the job makes before running cannot race the move.
func (d *Decay) post(actor DecayActor) {
	q := actor.Queue()
	if q == nil || d.postOn(q, actor) {
		return
	}
	if moved := actor.Queue(); moved != q && moved != nil {
		d.postOn(moved, actor)
	}
}

// postOn posts actor's decay to q, reporting whether q accepted it.
func (d *Decay) postOn(q *sim.Queue, actor DecayActor) bool {
	return q.Post(func() {
		if actor.Queue() != q {
			d.post(actor)
			return
		}
		d.effects.Decay(actor)
	})
}

func (d *Decay) cancelUnlinkedSummons() {
	d.mu.Lock()
	for key, entry := range d.entries {
		s, ok := entry.actor.(SummonDecayActor)
		if !ok || s.OwnerStillLinked() {
			continue
		}
		delete(d.entries, key)
	}
	d.mu.Unlock()
}
