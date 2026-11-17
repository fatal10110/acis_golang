package task

import (
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
)

// DoorTick is the fixed door-timer sweep interval.
const DoorTick = time.Second

// DoorEffects applies one scheduled door change once its deadline elapses.
type DoorEffects interface {
	SetDoorOpen(id int, open bool) bool
}

// doorChange is one scheduled change of a door to a fixed open state.
type doorChange struct {
	id       int
	open     bool
	deadline time.Time
}

// Door holds the doors' scheduled auto open/close changes and applies each
// one once its deadline elapses. A door may have several changes pending at
// once; each keeps the target state it was scheduled with and none replaces
// another.
//
// All methods are safe for concurrent use.
type Door struct {
	effects DoorEffects
	now     func() time.Time

	mu      sync.Mutex
	pending []doorChange // guarded by mu, in scheduling order
}

// NewDoor returns an empty door-timer tracker.
func NewDoor(effects DoorEffects, now func() time.Time) (*Door, error) {
	if effects == nil {
		return nil, errors.New("task: door effects is nil")
	}
	if now == nil {
		now = time.Now
	}
	return &Door{effects: effects, now: now}, nil
}

// Start launches the fixed one-second door-timer task.
func (d *Door) Start(log zerolog.Logger) *scheduler.Ticker {
	return scheduler.Start(DoorTick, d.Tick, log)
}

// Schedule queues a change of id's door to open, as open says, delay from
// now, alongside every change already pending for it. A delay of zero or
// less fires on the next Tick.
func (d *Door) Schedule(id int, open bool, delay time.Duration) {
	change := doorChange{id: id, open: open, deadline: d.now().Add(delay)}
	d.mu.Lock()
	d.pending = append(d.pending, change)
	d.mu.Unlock()
}

// Cancel drops every change pending for id and reports whether there was
// any.
func (d *Door) Cancel(id int) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := len(d.pending)
	d.pending = slices.DeleteFunc(d.pending, func(c doorChange) bool { return c.id == id })
	return len(d.pending) != n
}

// Tracked reports whether id has a change pending.
func (d *Door) Tracked(id int) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.ContainsFunc(d.pending, func(c doorChange) bool { return c.id == id })
}

// Tick applies every change whose deadline has passed, earliest deadline
// first and, between equal deadlines, in scheduling order. A change
// applied here may schedule another; it waits for a later Tick.
func (d *Door) Tick() {
	now := d.now()
	d.mu.Lock()
	var due []doorChange
	d.pending = slices.DeleteFunc(d.pending, func(c doorChange) bool {
		if now.Before(c.deadline) {
			return false
		}
		due = append(due, c)
		return true
	})
	d.mu.Unlock()

	slices.SortStableFunc(due, func(a, b doorChange) int { return a.deadline.Compare(b.deadline) })
	for _, c := range due {
		d.effects.SetDoorOpen(c.id, c.open)
	}
}
