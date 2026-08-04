package task

import (
	"errors"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
)

// AutosaveInitialDelay and AutosaveInterval reproduce GameClient's
// _autoSaveInDB schedule: first save 5 minutes after a player attaches,
// then every 15 minutes for as long as it stays tracked.
const (
	AutosaveInitialDelay = 5 * time.Minute
	AutosaveInterval     = 15 * time.Minute
	// AutosaveTick is the sweep granularity; coarse since deadlines are
	// minutes apart.
	AutosaveTick = 10 * time.Second
)

// AutosaveActor is the narrow actor surface the autosave task tracks.
type AutosaveActor interface {
	ObjectID() int32
}

// AutosaveEffects persists actor's full character state on each autosave
// tick.
type AutosaveEffects interface {
	Save(actor AutosaveActor)
}

type autosaveEntry struct {
	actor    AutosaveActor
	deadline time.Time
}

// Autosave periodically re-saves every tracked online player, matching the
// reference's per-connection autosave timer without a goroutine per
// connection.
//
// mu guards entries.
type Autosave struct {
	effects AutosaveEffects
	now     func() time.Time

	mu      sync.Mutex
	entries map[int32]autosaveEntry
}

// NewAutosave returns an empty autosave tracker that reports through
// effects.
func NewAutosave(effects AutosaveEffects, now func() time.Time) (*Autosave, error) {
	if effects == nil {
		return nil, errors.New("task: autosave effects is nil")
	}
	if now == nil {
		now = time.Now
	}
	return &Autosave{effects: effects, now: now, entries: make(map[int32]autosaveEntry)}, nil
}

// Start launches the fixed autosave sweep.
func (a *Autosave) Start(log zerolog.Logger) *scheduler.Ticker {
	return scheduler.Start(AutosaveTick, a.Tick, log)
}

// Add starts actor's autosave schedule, first firing after
// AutosaveInitialDelay. An already-tracked actor is left unchanged.
func (a *Autosave) Add(actor AutosaveActor) {
	if actor == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.entries[actor.ObjectID()]; ok {
		return
	}
	a.entries[actor.ObjectID()] = autosaveEntry{actor: actor, deadline: a.now().Add(AutosaveInitialDelay)}
}

// Remove stops actor's autosave schedule, if tracked.
func (a *Autosave) Remove(objectID int32) {
	a.mu.Lock()
	delete(a.entries, objectID)
	a.mu.Unlock()
}

// Tick saves every tracked actor whose deadline elapsed, then reschedules it
// AutosaveInterval out — repeating indefinitely until Remove.
func (a *Autosave) Tick() {
	now := a.now()
	a.mu.Lock()
	var due []AutosaveActor
	for id, entry := range a.entries {
		if now.Before(entry.deadline) {
			continue
		}
		due = append(due, entry.actor)
		entry.deadline = now.Add(AutosaveInterval)
		a.entries[id] = entry
	}
	a.mu.Unlock()

	for _, actor := range due {
		a.effects.Save(actor)
	}
}
