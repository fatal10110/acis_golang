package task

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
)

// DecayTick is the fixed corpse-decay sweep interval.
const DecayTick = time.Second

// DecayActor is the narrow actor surface tracked by the corpse decay task.
type DecayActor interface {
	ObjectID() int32
}

// DecayEffects delivers corpse-removal side effects when a tracked actor's
// decay deadline elapses.
type DecayEffects interface {
	Decay(actor DecayActor)
}

type decayEntry struct {
	actor    DecayActor
	deadline time.Time
}

// Decay tracks dead actors awaiting corpse removal and fires the removal
// side effect once each actor's display interval elapses.
//
// Add, Cancel, Tracked, and Deadline are safe to call concurrently with Tick.
// mu guards entries and the scratch refill. Tick only ever runs on the
// scheduler ticker's single goroutine, one call at a time; ticking enforces
// that contract by logging and returning ErrReentrantTick on reentrant or
// concurrent Tick calls instead of running, since Start's caller is the only
// reliable place that can act on the returned error.
type Decay struct {
	effects DecayEffects
	now     func() time.Time
	log     zerolog.Logger

	mu      sync.Mutex
	entries map[int32]decayEntry
	scratch []decayEntry

	ticking atomic.Bool
}

// NewDecay returns an empty corpse-decay tracker.
func NewDecay(effects DecayEffects, now func() time.Time) (*Decay, error) {
	if effects == nil {
		return nil, errors.New("task: decay effects is nil")
	}
	if now == nil {
		now = time.Now
	}
	return &Decay{effects: effects, now: now, entries: make(map[int32]decayEntry)}, nil
}

// Start launches the fixed one-second corpse-decay task.
func (d *Decay) Start(log zerolog.Logger) *scheduler.Ticker {
	d.log = log
	return scheduler.Start(DecayTick, func() { d.Tick() }, log)
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
	d.mu.Lock()
	d.entries[actor.ObjectID()] = decayEntry{actor: actor, deadline: deadline}
	d.mu.Unlock()
	return deadline
}

// Cancel stops tracking actor and reports whether it had been tracked.
func (d *Decay) Cancel(actor DecayActor) bool {
	if actor == nil {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	_, tracked := d.entries[actor.ObjectID()]
	if tracked {
		delete(d.entries, actor.ObjectID())
	}
	return tracked
}

// Tracked reports whether actor currently has a pending decay deadline.
func (d *Decay) Tracked(actor DecayActor) bool {
	if actor == nil {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.entries[actor.ObjectID()]
	return ok
}

// Deadline returns actor's pending decay deadline, if one is tracked.
func (d *Decay) Deadline(actor DecayActor) (time.Time, bool) {
	if actor == nil {
		return time.Time{}, false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	entry, ok := d.entries[actor.ObjectID()]
	if !ok {
		return time.Time{}, false
	}
	return entry.deadline, true
}

// Tick removes and decays every actor whose deadline has passed. It logs and
// returns ErrReentrantTick without doing anything else if another Tick call
// is already in flight.
func (d *Decay) Tick() error {
	if !d.ticking.CompareAndSwap(false, true) {
		d.log.Error().Err(ErrReentrantTick).Msg("task: Decay.Tick")
		return ErrReentrantTick
	}
	defer d.ticking.Store(false)

	now := d.now()

	d.mu.Lock()
	d.scratch = d.scratch[:0]
	for id, entry := range d.entries {
		if now.Before(entry.deadline) {
			continue
		}
		d.scratch = append(d.scratch, entry)
		delete(d.entries, id)
	}
	due := d.scratch
	d.mu.Unlock()

	defer clear(d.scratch)

	for _, entry := range due {
		d.effects.Decay(entry.actor)
	}
	return nil
}
