package task

import (
	"errors"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
)

// RespawnTick is the fixed spawn-slot respawn sweep interval.
const RespawnTick = time.Second

// RespawnEffects re-instantiates one spawn slot once its respawn deadline
// elapses. Slots are identified by a caller-owned string key rather than an
// object id, since a slot waiting to respawn has no live world object yet.
type RespawnEffects interface {
	Respawn(key string)
}

type respawnEntry struct {
	deadline time.Time
}

// Respawn tracks spawn slots awaiting their next respawn deadline and fires
// the respawn side effect once each slot's deadline elapses.
//
// All methods are safe for concurrent use; mu guards entries.
type Respawn struct {
	effects RespawnEffects
	now     func() time.Time

	mu      sync.Mutex
	entries map[string]respawnEntry
}

// NewRespawn returns an empty spawn-slot respawn tracker.
func NewRespawn(effects RespawnEffects, now func() time.Time) (*Respawn, error) {
	if effects == nil {
		return nil, errors.New("task: respawn effects is nil")
	}
	if now == nil {
		now = time.Now
	}
	return &Respawn{effects: effects, now: now, entries: make(map[string]respawnEntry)}, nil
}

// Start launches the fixed one-second respawn task.
func (r *Respawn) Start(log zerolog.Logger) *scheduler.Ticker {
	return scheduler.Start(RespawnTick, r.Tick, log)
}

// Add schedules key to respawn at deadline, replacing any deadline already
// tracked for it. A deadline at or before now fires on the next Tick.
func (r *Respawn) Add(key string, deadline time.Time) {
	r.mu.Lock()
	r.entries[key] = respawnEntry{deadline: deadline}
	r.mu.Unlock()
}

// Cancel stops tracking key and reports whether it had been tracked.
func (r *Respawn) Cancel(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, tracked := r.entries[key]
	if tracked {
		delete(r.entries, key)
	}
	return tracked
}

// Tracked reports whether key currently has a pending respawn deadline.
func (r *Respawn) Tracked(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.entries[key]
	return ok
}

// Tick respawns every slot whose deadline has passed.
func (r *Respawn) Tick() {
	now := r.now()

	r.mu.Lock()
	due := make([]string, 0, len(r.entries))
	for key, entry := range r.entries {
		if now.Before(entry.deadline) {
			continue
		}
		due = append(due, key)
		delete(r.entries, key)
	}
	r.mu.Unlock()

	for _, key := range due {
		r.effects.Respawn(key)
	}
}
