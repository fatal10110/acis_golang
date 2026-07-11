package task

import (
	"errors"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
)

// WaterTick is the fixed drowning-check interval.
const WaterTick = time.Second

// WaterActor is the narrow player surface the water task needs to track a
// drowning countdown.
type WaterActor interface {
	ObjectID() int32
	Dead() bool
}

// WaterEffects delivers the water task's client-visible side effects. The
// breath-timer duration and drowning-damage amount depend on the actor's
// stats (max HP, race, class) and packet encoding depends on the network
// layer, so the caller supplies both.
type WaterEffects interface {
	// GaugeSet reports actor's breath countdown; remaining is 0 when the
	// countdown ends (actor surfaced, died, or was otherwise removed).
	GaugeSet(actor WaterActor, remaining time.Duration)
	// Drown applies one tick's worth of drowning damage to actor. Called
	// once per tick for every actor past its breath limit, repeatedly,
	// until the actor is removed.
	Drown(actor WaterActor)
}

type waterEntry struct {
	actor    WaterActor
	deadline time.Time
}

// Water tracks the drowning countdown for actors submerged past their
// breath limit and signals drowning damage every tick until they surface,
// die, or are otherwise removed.
//
// mu guards entries.
type Water struct {
	effects WaterEffects
	now     func() time.Time

	mu      sync.Mutex
	entries map[int32]waterEntry
}

// NewWater returns an empty drowning tracker that reports through effects.
func NewWater(effects WaterEffects, now func() time.Time) (*Water, error) {
	if effects == nil {
		return nil, errors.New("task: water effects is nil")
	}
	if now == nil {
		now = time.Now
	}
	return &Water{
		effects: effects,
		now:     now,
		entries: make(map[int32]waterEntry),
	}, nil
}

// Start launches the fixed one-second drowning task.
func (w *Water) Start(log zerolog.Logger) *scheduler.Ticker {
	return scheduler.Start(WaterTick, w.Tick, log)
}

// Add starts actor's breath countdown, submerged for breath before
// drowning damage begins. A dead or already-tracked actor is left
// unchanged.
func (w *Water) Add(actor WaterActor, breath time.Duration) {
	if actor == nil || actor.Dead() {
		return
	}

	w.mu.Lock()
	if _, ok := w.entries[actor.ObjectID()]; ok {
		w.mu.Unlock()
		return
	}
	w.entries[actor.ObjectID()] = waterEntry{actor: actor, deadline: w.now().Add(breath)}
	w.mu.Unlock()

	w.effects.GaugeSet(actor, breath)
}

// Remove stops tracking actor's breath countdown, if it was tracked.
func (w *Water) Remove(actor WaterActor) {
	if actor == nil {
		return
	}

	w.mu.Lock()
	_, tracked := w.entries[actor.ObjectID()]
	if tracked {
		delete(w.entries, actor.ObjectID())
	}
	w.mu.Unlock()

	if tracked {
		w.effects.GaugeSet(actor, 0)
	}
}

// Tick signals drowning damage for every tracked actor whose breath
// countdown has elapsed. An actor stays past its deadline (and keeps
// drowning every tick) until Remove is called.
func (w *Water) Tick() {
	w.mu.Lock()
	var due []WaterActor
	now := w.now()
	for _, entry := range w.entries {
		if now.Before(entry.deadline) {
			continue
		}
		due = append(due, entry.actor)
	}
	w.mu.Unlock()

	for _, actor := range due {
		w.effects.Drown(actor)
	}
}
