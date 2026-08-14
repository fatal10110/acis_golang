package task

import (
	"errors"
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

// Respawn tracks spawn slots awaiting their next respawn deadline and fires
// the respawn side effect once each slot's deadline elapses.
//
// All methods are safe for concurrent use.
type Respawn struct {
	effects RespawnEffects
	now     func() time.Time

	*deadlineRegistry[string, string]
}

// NewRespawn returns an empty spawn-slot respawn tracker.
func NewRespawn(effects RespawnEffects, now func() time.Time) (*Respawn, error) {
	if effects == nil {
		return nil, errors.New("task: respawn effects is nil")
	}
	if now == nil {
		now = time.Now
	}
	return &Respawn{effects: effects, now: now, deadlineRegistry: newDeadlineRegistry[string, string]()}, nil
}

// Start launches the fixed one-second respawn task.
func (r *Respawn) Start(log zerolog.Logger) *scheduler.Ticker {
	return scheduler.Start(RespawnTick, r.Tick, log)
}

// Add schedules key to respawn at deadline, replacing any deadline already
// tracked for it. A deadline at or before now fires on the next Tick.
func (r *Respawn) Add(key string, deadline time.Time) {
	r.add(key, key, deadline)
}

// Cancel stops tracking key and reports whether it had been tracked.
func (r *Respawn) Cancel(key string) bool {
	return r.remove(key)
}

// Tracked reports whether key currently has a pending respawn deadline.
func (r *Respawn) Tracked(key string) bool {
	return r.tracked(key)
}

// Tick respawns every slot whose deadline has passed.
func (r *Respawn) Tick() {
	r.tickDueConcurrent(r.now(), r.effects.Respawn)
}
