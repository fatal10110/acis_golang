package cubic

import (
	"reflect"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
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

// newRuntimeClock returns an owner queue on a virtual clock that moves only
// on Advance.
func newRuntimeClock() (*sim.Inline, *sim.Queue) {
	in := sim.NewInline(time.Unix(1000, 0))
	return in, in.NewQueue("owner")
}

func TestRuntime_ActionSchedulesAndReschedulesTick(t *testing.T) {
	in, q := newRuntimeClock()
	fireCount := 0
	r := NewRuntime(Storm, 3, 30, 5*time.Second, func() { fireCount++ }, func() {}, q)

	r.Action()
	in.Advance(5*time.Second - time.Millisecond)
	if fireCount != 0 {
		t.Fatalf("fireCount before the first 5s tick = %d, want 0", fireCount)
	}
	in.Advance(time.Millisecond)
	if fireCount != 1 {
		t.Fatalf("fireCount after one tick = %d, want 1", fireCount)
	}
	in.Advance(5 * time.Second)
	if fireCount != 2 {
		t.Fatalf("fireCount after two ticks = %d, want 2 (tick reschedules itself)", fireCount)
	}
}

func TestRuntime_ActionIsIdempotent(t *testing.T) {
	in, q := newRuntimeClock()
	fireCount := 0
	r := NewRuntime(Storm, 1, 30, time.Second, func() { fireCount++ }, func() {}, q)

	// A second Action() halfway through the interval must not restart it.
	r.Action()
	in.Advance(500 * time.Millisecond)
	r.Action()
	in.Advance(500 * time.Millisecond)
	if fireCount != 1 {
		t.Fatalf("fireCount one interval after the first Action() = %d, want 1 (a repeat Action() does not restart the tick)", fireCount)
	}
}

func TestRuntime_StopActionCancelsAndActionRestarts(t *testing.T) {
	in, q := newRuntimeClock()
	fireCount := 0
	r := NewRuntime(Storm, 1, 30, time.Second, func() { fireCount++ }, func() {}, q)

	r.Action()
	r.StopAction()
	in.Advance(3 * time.Second)
	if fireCount != 0 {
		t.Fatalf("fireCount after StopAction = %d, want 0", fireCount)
	}

	r.Action()
	in.Advance(time.Second)
	if fireCount != 1 {
		t.Fatalf("fireCount after Action() restarted it = %d, want 1", fireCount)
	}
}

// TestRuntime_TickRecoversPanicAndAllowsActionToRestart proves a panicking
// fire() doesn't leave running stuck true: without the reset, Action()'s
// no-op-if-already-active guard would permanently block every future
// stance re-entry, silently stalling the cubic for the rest of its grant.
// The panic must still reach the caller (in production the tick runs as a
// task on the owner's queue, and the pool's per-task recovery logs it)
// rather than being swallowed here.
func TestRuntime_TickRecoversPanicAndAllowsActionToRestart(t *testing.T) {
	in, q := newRuntimeClock()
	fireCount := 0
	r := NewRuntime(Storm, 1, 30, time.Second, func() {
		fireCount++
		if fireCount == 1 {
			panic("boom")
		}
	}, func() {}, q)

	r.Action()

	func() {
		defer func() {
			if p := recover(); p == nil {
				t.Fatal("tick did not propagate the panic to the caller")
			} else if p != "boom" {
				t.Fatalf("recovered panic = %v, want boom", p)
			}
		}()
		in.Advance(time.Second)
	}()

	in.Advance(3 * time.Second)
	if fireCount != 1 {
		t.Fatalf("fireCount after a panicking tick = %d, want 1 (no reschedule)", fireCount)
	}

	// Action() must be able to restart it after the panic, like it can
	// after StopAction() — otherwise the cubic stays dead until expiry.
	r.Action()
	in.Advance(time.Second)
	if fireCount != 2 {
		t.Fatalf("fireCount after restart = %d, want 2", fireCount)
	}
}

func TestRuntime_RefreshDisappearReplacesPendingTimer(t *testing.T) {
	in, q := newRuntimeClock()
	disappeared := 0
	r := NewRuntime(Life, 1, 30, time.Second, func() {}, func() { disappeared++ }, q)

	r.RefreshDisappear(10 * time.Second)
	r.RefreshDisappear(20 * time.Second)

	in.Advance(20*time.Second - time.Millisecond)
	if disappeared != 0 {
		t.Fatalf("disappeared before the refreshed 20s lifetime = %d, want 0 (10s timer cancelled)", disappeared)
	}
	in.Advance(time.Millisecond)
	if disappeared != 1 {
		t.Fatalf("disappeared = %d, want 1", disappeared)
	}
}

func TestRuntime_StopCancelsBothTimers(t *testing.T) {
	in, q := newRuntimeClock()
	fireCount, disappeared := 0, 0
	r := NewRuntime(Storm, 1, 30, time.Second, func() { fireCount++ }, func() { disappeared++ }, q)

	r.Action()
	r.RefreshDisappear(time.Minute)
	r.Stop()
	in.Advance(2 * time.Minute)

	if fireCount != 0 || disappeared != 0 {
		t.Fatalf("after Stop(): fireCount = %d, disappeared = %d; want 0, 0", fireCount, disappeared)
	}
}

// TestRuntime_StopActionThenActionDuringFireDoesNotOrphanATimer covers
// StopAction() immediately followed by Action() while a tick's fire() is
// still running. Without the generation guard, the in-flight tick's own
// post-fire reschedule would run on r.running alone and arm a second timer
// in place of the one Action() just armed, orphaning it.
func TestRuntime_StopActionThenActionDuringFireDoesNotOrphanATimer(t *testing.T) {
	in, q := newRuntimeClock()
	fireCount := 0
	var restarted *sim.Timer
	var r *Runtime
	r = NewRuntime(Storm, 1, 30, time.Second, func() {
		fireCount++
		if fireCount == 1 {
			r.StopAction()
			r.Action()
			r.mu.Lock()
			restarted = r.actionTimer
			r.mu.Unlock()
		}
	}, func() {}, q)

	r.Action()
	in.Advance(time.Second) // first tick: fire() itself calls StopAction()+Action()

	r.mu.Lock()
	current := r.actionTimer
	r.mu.Unlock()
	if current != restarted {
		t.Fatal("the in-flight tick replaced the timer Action() armed inside fire()")
	}
	in.Advance(time.Second)
	if fireCount != 2 {
		t.Fatalf("fireCount after the restarted chain's first tick = %d, want 2", fireCount)
	}
}

func TestRuntime_ID(t *testing.T) {
	r := NewRuntime(Vampiric, 1, 30, time.Second, func() {}, func() {}, nil)
	if r.ID() != int(Vampiric) {
		t.Fatalf("ID() = %d, want %d", r.ID(), int(Vampiric))
	}
}
