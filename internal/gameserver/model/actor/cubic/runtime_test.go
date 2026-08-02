package cubic

import (
	"testing"
	"time"
)

type fakeTimer struct {
	stopped bool
}

func (t *fakeTimer) Stop() bool {
	wasRunning := !t.stopped
	t.stopped = true
	return wasRunning
}

// fakeClock is a deterministic AfterFunc stand-in: each scheduled call is
// recorded but never runs on its own. Tests advance time explicitly by
// invoking the recorded callback, mirroring the injected afterFunc clock
// seam pattern used across this port's other scheduled state machines.
type fakeClock struct {
	scheduled []fakeSchedule
}

type fakeSchedule struct {
	delay time.Duration
	fn    func()
	timer *fakeTimer
}

func (c *fakeClock) after(d time.Duration, fn func()) Timer {
	t := &fakeTimer{}
	c.scheduled = append(c.scheduled, fakeSchedule{delay: d, fn: fn, timer: t})
	return t
}

// fireLast invokes the most recently scheduled, still-pending callback, as
// Runtime.tick reschedules on every fire.
func (c *fakeClock) fireLast() {
	if len(c.scheduled) == 0 {
		return
	}
	c.scheduled[len(c.scheduled)-1].fn()
}

func TestRuntime_ActionSchedulesAndReschedulesTick(t *testing.T) {
	clock := &fakeClock{}
	fireCount := 0
	r := NewRuntime(Storm, 3, 30, 5*time.Second, func() { fireCount++ }, func() {}, clock.after)

	r.Action()
	if len(clock.scheduled) != 1 || clock.scheduled[0].delay != 5*time.Second {
		t.Fatalf("Action() scheduled = %+v, want one 5s tick", clock.scheduled)
	}

	clock.fireLast()
	if fireCount != 1 {
		t.Fatalf("fireCount after one tick = %d, want 1", fireCount)
	}
	if len(clock.scheduled) != 2 {
		t.Fatalf("tick did not reschedule: scheduled = %d, want 2", len(clock.scheduled))
	}

	clock.fireLast()
	if fireCount != 2 {
		t.Fatalf("fireCount after two ticks = %d, want 2", fireCount)
	}
}

func TestRuntime_ActionIsIdempotent(t *testing.T) {
	clock := &fakeClock{}
	r := NewRuntime(Storm, 1, 30, time.Second, func() {}, func() {}, clock.after)

	r.Action()
	r.Action()
	if len(clock.scheduled) != 1 {
		t.Fatalf("Action() called twice scheduled %d timers, want 1 (idempotent)", len(clock.scheduled))
	}
}

func TestRuntime_StopActionCancelsAndTickDoesNothingAfter(t *testing.T) {
	clock := &fakeClock{}
	fireCount := 0
	r := NewRuntime(Storm, 1, 30, time.Second, func() { fireCount++ }, func() {}, clock.after)

	r.Action()
	r.StopAction()
	if !clock.scheduled[0].timer.stopped {
		t.Fatal("StopAction() did not stop the action timer")
	}

	// A tick already in flight when StopAction ran must still no-op rather
	// than fire or reschedule.
	clock.fireLast()
	if fireCount != 0 {
		t.Fatalf("fireCount after StopAction = %d, want 0", fireCount)
	}
	if len(clock.scheduled) != 1 {
		t.Fatalf("scheduled after a no-op tick = %d, want 1 (no reschedule)", len(clock.scheduled))
	}

	// Action() must be able to restart it afterward.
	r.Action()
	if len(clock.scheduled) != 2 {
		t.Fatalf("Action() after StopAction() did not reschedule: scheduled = %d, want 2", len(clock.scheduled))
	}
}

func TestRuntime_RefreshDisappearReplacesPendingTimer(t *testing.T) {
	clock := &fakeClock{}
	disappeared := 0
	r := NewRuntime(Life, 1, 30, time.Second, func() {}, func() { disappeared++ }, clock.after)

	r.RefreshDisappear(10 * time.Second)
	first := clock.scheduled[0].timer
	r.RefreshDisappear(20 * time.Second)

	if !first.stopped {
		t.Fatal("RefreshDisappear() did not cancel the previous disappear timer")
	}
	if len(clock.scheduled) != 2 || clock.scheduled[1].delay != 20*time.Second {
		t.Fatalf("RefreshDisappear() scheduling = %+v, want a fresh 20s timer", clock.scheduled)
	}

	clock.fireLast()
	if disappeared != 1 {
		t.Fatalf("disappeared = %d, want 1", disappeared)
	}
}

func TestRuntime_StopCancelsBothTimers(t *testing.T) {
	clock := &fakeClock{}
	r := NewRuntime(Storm, 1, 30, time.Second, func() {}, func() {}, clock.after)

	r.Action()
	r.RefreshDisappear(time.Minute)
	r.Stop()

	for i, s := range clock.scheduled {
		if !s.timer.stopped {
			t.Fatalf("scheduled[%d] not stopped after Stop()", i)
		}
	}
}

// TestRuntime_StopActionThenActionDuringFireDoesNotOrphanATimer reproduces
// the race where StopAction() immediately followed by Action() happens
// while a tick's fire() callback is still running (a realistic sequence
// since the timer callback and an attack-stance-driven Action()/
// StopAction() call run on different goroutines in production). Without the
// generation guard, the in-flight tick's own post-fire reschedule would run
// unconditionally on r.running alone, creating a second independent timer
// chain alongside the one Action() just started inside fire().
func TestRuntime_StopActionThenActionDuringFireDoesNotOrphanATimer(t *testing.T) {
	clock := &fakeClock{}
	fireCount := 0
	var r *Runtime
	r = NewRuntime(Storm, 1, 30, time.Second, func() {
		fireCount++
		if fireCount == 1 {
			r.StopAction()
			r.Action()
		}
	}, func() {}, clock.after)

	r.Action()
	clock.fireLast() // first tick: fire() itself calls StopAction()+Action()

	live := 0
	for _, s := range clock.scheduled {
		if !s.timer.stopped {
			live++
		}
	}
	if live != 1 {
		t.Fatalf("live (unstopped) scheduled timers after StopAction()+Action() inside fire() = %d, want 1 (no orphaned duplicate chain)", live)
	}

	// The stale tick (if any survived) must not have rescheduled a second
	// time from its own post-fire check; only Action()'s own fresh timer
	// should still be pending.
	clock.fireLast()
	if fireCount != 2 {
		t.Fatalf("fireCount after second fireLast() = %d, want 2 (single active chain, no duplicate fire rate)", fireCount)
	}
}

func TestRuntime_ID(t *testing.T) {
	r := NewRuntime(Vampiric, 1, 30, time.Second, func() {}, func() {}, nil)
	if r.ID() != int(Vampiric) {
		t.Fatalf("ID() = %d, want %d", r.ID(), int(Vampiric))
	}
}
