package cubic

import (
	"reflect"
	"testing"
	"time"
)

// ---- from cubic_test.go ----
func TestSkillIDs(t *testing.T) {
	tests := []struct {
		id   ID
		want []int
	}{
		{Storm, []int{4049}},
		{Poltergeist, []int{4053, 4054, 4055}},
		{Attract, []int{5115, 5116}},
		{ID(99), nil},
	}
	for _, tt := range tests {
		if got := SkillIDs(tt.id); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("SkillIDs(%v) = %v, want %v", tt.id, got, tt.want)
		}
	}
}

func TestList_AddOrRefresh(t *testing.T) {
	var l List

	refreshed, _, evicted := l.AddOrRefresh(Storm, false, 2)
	if refreshed || evicted {
		t.Fatalf("first add: refreshed=%v evicted=%v, want false,false", refreshed, evicted)
	}
	if !l.Has(Storm) || l.Len() != 1 {
		t.Fatalf("after first add: Has(Storm)=%v Len=%d, want true,1", l.Has(Storm), l.Len())
	}

	// Re-adding the same id reports a refresh and changes nothing.
	refreshed, _, evicted = l.AddOrRefresh(Storm, false, 2)
	if !refreshed || evicted {
		t.Fatalf("re-add same id: refreshed=%v evicted=%v, want true,false", refreshed, evicted)
	}
	if l.Len() != 1 {
		t.Fatalf("Len after refresh = %d, want 1", l.Len())
	}
}

func TestList_AddOrRefresh_EvictsOldestPastCap(t *testing.T) {
	var l List
	maxSlots := 1 // isFull is size > maxSlots, so a 2nd add before this cap doesn't evict

	l.AddOrRefresh(Storm, false, maxSlots)
	refreshed, evicted, didEvict := l.AddOrRefresh(Vampiric, false, maxSlots)
	if refreshed || didEvict {
		t.Fatalf("2nd add at size 1 > maxSlots 1 is false: refreshed=%v didEvict=%v, want false,false", refreshed, didEvict)
	}
	if l.Len() != 2 {
		t.Fatalf("Len after 2nd add = %d, want 2", l.Len())
	}

	// Now size (2) > maxSlots (1): the next add evicts the oldest entry
	// (Storm) before admitting the new one.
	refreshed, evicted, didEvict = l.AddOrRefresh(Life, false, maxSlots)
	if refreshed || !didEvict || evicted != Storm {
		t.Fatalf("3rd add: refreshed=%v didEvict=%v evicted=%v, want false,true,Storm", refreshed, didEvict, evicted)
	}
	if l.Has(Storm) {
		t.Errorf("Storm should have been evicted")
	}
	if !l.Has(Vampiric) || !l.Has(Life) {
		t.Errorf("Vampiric and Life should both remain active")
	}
	if l.Len() != 2 {
		t.Fatalf("Len after eviction = %d, want 2", l.Len())
	}
}

func TestList_IDs(t *testing.T) {
	var l List
	if got := l.IDs(); len(got) != 0 {
		t.Fatalf("IDs() on empty list = %v, want empty", got)
	}

	l.AddOrRefresh(Vampiric, false, 5)
	l.AddOrRefresh(Storm, false, 5)

	want := []int{int(Vampiric), int(Storm)}
	if got := l.IDs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("IDs() = %v, want %v (grant order)", got, want)
	}
}

func TestList_Remove(t *testing.T) {
	var l List
	l.AddOrRefresh(Storm, false, 5)
	l.AddOrRefresh(Vampiric, false, 5)

	l.Remove(Storm)
	if l.Has(Storm) {
		t.Errorf("Storm should have been removed")
	}
	if !l.Has(Vampiric) {
		t.Errorf("Vampiric should remain")
	}

	// Removing an id that isn't active is a no-op.
	l.Remove(Storm)
	if l.Len() != 1 {
		t.Errorf("Len after removing an absent id = %d, want 1", l.Len())
	}
}

func TestList_StopAll(t *testing.T) {
	var l List
	l.AddOrRefresh(Storm, false, 5)
	l.AddOrRefresh(Vampiric, true, 5)

	stopped := l.StopAll()
	if len(stopped) != 2 {
		t.Fatalf("StopAll() returned %d ids, want 2", len(stopped))
	}
	if l.Len() != 0 {
		t.Errorf("Len after StopAll = %d, want 0", l.Len())
	}
}

func TestList_StopGivenByOthers(t *testing.T) {
	var l List
	l.AddOrRefresh(Storm, false, 5)   // own cubic
	l.AddOrRefresh(Vampiric, true, 5) // granted by a party member
	l.AddOrRefresh(Life, true, 5)     // also granted

	stopped := l.StopGivenByOthers()
	if len(stopped) != 2 {
		t.Fatalf("StopGivenByOthers() returned %d ids, want 2", len(stopped))
	}
	if !l.Has(Storm) {
		t.Errorf("owner's own cubic should remain active")
	}
	if l.Has(Vampiric) || l.Has(Life) {
		t.Errorf("cubics granted by others should have been stopped")
	}
	if l.Len() != 1 {
		t.Errorf("Len after StopGivenByOthers = %d, want 1", l.Len())
	}
}

// ---- from runtime_test.go ----
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

// TestRuntime_TickRecoversPanicAndAllowsActionToRestart proves a panicking
// fire() doesn't leave running stuck true: without the reset, Action()'s
// no-op-if-already-active guard would permanently block every future
// stance re-entry, silently stalling the cubic for the rest of its grant.
// The panic must still reach the caller (recovered/logged by the
// production afterFunc, e.g. GameClientLink's cubicAfterFunc) rather than
// being swallowed here.
func TestRuntime_TickRecoversPanicAndAllowsActionToRestart(t *testing.T) {
	clock := &fakeClock{}
	fireCount := 0
	r := NewRuntime(Storm, 1, 30, time.Second, func() {
		fireCount++
		if fireCount == 1 {
			panic("boom")
		}
	}, func() {}, clock.after)

	r.Action()

	func() {
		defer func() {
			if p := recover(); p == nil {
				t.Fatal("tick did not propagate the panic to the caller")
			} else if p != "boom" {
				t.Fatalf("recovered panic = %v, want boom", p)
			}
		}()
		clock.fireLast()
	}()

	if len(clock.scheduled) != 1 {
		t.Fatalf("scheduled after a panicking tick = %d, want 1 (no reschedule)", len(clock.scheduled))
	}

	// Action() must be able to restart it after the panic, like it can
	// after StopAction() — otherwise the cubic stays dead until expiry.
	r.Action()
	if len(clock.scheduled) != 2 {
		t.Fatalf("Action() after a recovered panic did not restart: scheduled = %d, want 2", len(clock.scheduled))
	}

	clock.fireLast()
	if fireCount != 2 {
		t.Fatalf("fireCount after restart = %d, want 2", fireCount)
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
