package task

import (
	"errors"
	"sync"
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
// All methods are safe for concurrent use; mu guards entries.
type Decay struct {
	effects DecayEffects
	now     func() time.Time

	mu      sync.Mutex
	entries map[int32]decayEntry
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
	return scheduler.Start(DecayTick, d.Tick, log)
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

// Tick removes and decays every actor whose deadline has passed.
func (d *Decay) Tick() {
	now := d.now()

	d.mu.Lock()
	due := make([]decayEntry, 0, len(d.entries))
	for id, entry := range d.entries {
		if now.Before(entry.deadline) {
			continue
		}
		due = append(due, entry)
		delete(d.entries, id)
	}
	d.mu.Unlock()

	for _, entry := range due {
		d.effects.Decay(entry.actor)
	}
}
