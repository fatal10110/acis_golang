package scheduler

import (
	"sync"
	"time"
)

// Timer is a scheduled callback that can be cancelled before it runs.
type Timer interface {
	Stop() bool
}

// Clock supplies time and delayed callbacks. Production code may use RealClock;
// tests can advance a ManualClock without waiting for wall time.
type Clock interface {
	Now() time.Time
	AfterFunc(time.Duration, func()) Timer
}

// RealClock delegates to the process clock.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

func (RealClock) AfterFunc(delay time.Duration, fn func()) Timer { return time.AfterFunc(delay, fn) }

// ManualClock is a deterministic clock for a single test server.
type ManualClock struct {
	mu     sync.Mutex
	now    time.Time
	nextID uint64
	timers []*manualTimer
}

type manualTimer struct {
	clock    *ManualClock
	deadline time.Time
	id       uint64
	fn       func()
	stopped  bool
	fired    bool
}

// NewManualClock returns a clock beginning at now.
func NewManualClock(now time.Time) *ManualClock { return &ManualClock{now: now} }

func (c *ManualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *ManualClock) AfterFunc(delay time.Duration, fn func()) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	if delay < 0 {
		delay = 0
	}
	c.nextID++
	t := &manualTimer{clock: c, deadline: c.now.Add(delay), id: c.nextID, fn: fn}
	c.timers = append(c.timers, t)
	return t
}

func (t *manualTimer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	if t.stopped || t.fired {
		return false
	}
	t.stopped = true
	return true
}

// Advance moves the clock and runs every due callback in deadline then
// registration order. Callbacks execute without holding the clock lock.
func (c *ManualClock) Advance(d time.Duration) {
	c.mu.Lock()
	target := c.now.Add(d)
	c.mu.Unlock()

	for {
		c.mu.Lock()
		var due *manualTimer
		for _, candidate := range c.timers {
			if candidate.stopped || candidate.fired || candidate.deadline.After(target) {
				continue
			}
			if due == nil || candidate.deadline.Before(due.deadline) || candidate.deadline.Equal(due.deadline) && candidate.id < due.id {
				due = candidate
			}
		}
		if due != nil {
			due.fired = true
			c.now = due.deadline
		} else {
			c.now = target
		}
		c.mu.Unlock()
		if due == nil {
			return
		}
		due.fn()
	}
}

// Pending returns scheduled callbacks that have not fired or been stopped.
// It is useful only in tests.
func (c *ManualClock) Pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	count := 0
	for _, timer := range c.timers {
		if !timer.stopped && !timer.fired {
			count++
		}
	}
	return count
}

var _ Clock = RealClock{}
var _ Clock = (*ManualClock)(nil)
var _ Timer = (*manualTimer)(nil)
