package task

import (
	"errors"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
)

// DoorTick is the fixed door-timer sweep interval.
const DoorTick = time.Second

// DoorEffects flips one door's open state once its scheduled deadline
// elapses.
type DoorEffects interface {
	ToggleDoor(id int)
}

// Door tracks doors awaiting their next scheduled open/close transition and
// fires the toggle side effect once each door's deadline elapses.
//
// All methods are safe for concurrent use.
type Door struct {
	effects DoorEffects
	now     func() time.Time

	*deadlineRegistry[int, int]
}

// NewDoor returns an empty door-timer tracker.
func NewDoor(effects DoorEffects, now func() time.Time) (*Door, error) {
	if effects == nil {
		return nil, errors.New("task: door effects is nil")
	}
	if now == nil {
		now = time.Now
	}
	return &Door{effects: effects, now: now, deadlineRegistry: newDeadlineRegistry[int, int]()}, nil
}

// Start launches the fixed one-second door-timer task.
func (d *Door) Start(log zerolog.Logger) *scheduler.Ticker {
	return scheduler.Start(DoorTick, d.Tick, log)
}

// Add schedules id to toggle at deadline, replacing any deadline already
// tracked for it. A deadline at or before now fires on the next Tick.
func (d *Door) Add(id int, deadline time.Time) {
	d.add(id, id, deadline)
}

// Cancel stops tracking id and reports whether it had been tracked.
func (d *Door) Cancel(id int) bool {
	return d.remove(id)
}

// Tracked reports whether id currently has a pending toggle deadline.
func (d *Door) Tracked(id int) bool {
	return d.tracked(id)
}

// Tick toggles every door whose deadline has passed.
func (d *Door) Tick() {
	d.tickDueConcurrent(d.now(), d.effects.ToggleDoor)
}
