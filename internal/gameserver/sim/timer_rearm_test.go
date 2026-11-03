package sim

import (
	"testing"
	"time"
)

// TestTimerRearmRunsAgainAtTheNewDeadline: a one-shot re-armed from its fn,
// or after it ran, runs again d after the re-arm; one that has not run yet
// refuses.
func TestTimerRearmRunsAgainAtTheNewDeadline(t *testing.T) {
	in := NewInline(epoch)
	q := in.NewQueue("q")
	var ran []time.Duration
	var tm *Timer
	tm = q.After(10*time.Millisecond, func() {
		ran = append(ran, in.Now().Sub(epoch))
		if len(ran) < 3 && !tm.Rearm(5*time.Millisecond) {
			t.Error("Rearm from fn refused")
		}
	})
	if tm.Rearm(time.Millisecond) {
		t.Fatal("Rearm of a timer that has not run accepted")
	}
	in.Advance(time.Second)
	if !tm.Rearm(time.Millisecond) {
		t.Fatal("Rearm after fn ran refused")
	}
	if tm.Rearm(time.Millisecond) {
		t.Fatal("second Rearm of an armed timer accepted")
	}
	in.Advance(time.Second)

	want := []time.Duration{10 * time.Millisecond, 15 * time.Millisecond, 20 * time.Millisecond, 1001 * time.Millisecond}
	if len(ran) != len(want) {
		t.Fatalf("ran at %v, want %v", ran, want)
	}
	for i := range want {
		if ran[i] != want[i] {
			t.Fatalf("ran at %v, want %v", ran, want)
		}
	}
}

// TestTimerRearmStopAndClose: Stop and Close cancel a re-armed timer, and a
// timer stopped, or whose queue closed, after it ran stays dead; a timer
// AfterPastClose armed re-arms on a closed queue.
func TestTimerRearmStopAndClose(t *testing.T) {
	in := NewInline(epoch)
	ranOnce := func(q *Queue, pastClose bool) (*Timer, *int) {
		n := new(int)
		arm := q.After
		if pastClose {
			arm = q.AfterPastClose
		}
		tm := arm(time.Millisecond, func() { *n++ })
		in.Advance(time.Millisecond)
		if *n != 1 {
			t.Fatalf("timer ran %d times, want 1", *n)
		}
		return tm, n
	}

	q := in.NewQueue("stop")
	tm, n := ranOnce(q, false)
	if !tm.Rearm(time.Millisecond) || !tm.Stop() {
		t.Fatal("Stop did not cancel the re-armed timer")
	}
	if tm.Rearm(time.Millisecond) {
		t.Fatal("Rearm after Stop accepted")
	}
	tm2, n2 := ranOnce(q, false)
	if tm2.Stop() {
		t.Fatal("Stop of a ran one-shot reported a cancel")
	}
	if tm2.Rearm(time.Millisecond) {
		t.Fatal("Rearm of a ran one-shot stopped since accepted")
	}

	c := in.NewQueue("close")
	tm3, n3 := ranOnce(c, false)
	tm4, n4 := ranOnce(c, false)
	tm5, n5 := ranOnce(c, true)
	if !tm3.Rearm(time.Millisecond) {
		t.Fatal("Rearm on an open queue refused")
	}
	c.Close()
	if tm4.Rearm(time.Millisecond) {
		t.Fatal("Rearm on a closed queue accepted")
	}
	if !tm5.Rearm(time.Millisecond) {
		t.Fatal("Rearm of an AfterPastClose timer on a closed queue refused")
	}
	in.Advance(time.Second)

	for i, c := range []struct{ got, want int }{{*n, 1}, {*n2, 1}, {*n3, 1}, {*n4, 1}, {*n5, 2}} {
		if c.got != c.want {
			t.Errorf("timer %d ran %d times, want %d", i, c.got, c.want)
		}
	}
}

// TestTimerRearmAllocatesNothing: once warm, an expiry that re-arms its
// timer allocates nothing, the clock's own firing included.
func TestTimerRearmAllocatesNothing(t *testing.T) {
	if simdebugBuild {
		t.Skip("simdebug owner bookkeeping allocates per drain; the budget describes the production build")
	}
	in := NewInline(epoch)
	q := in.NewQueue("q")
	var tm *Timer
	tm = q.After(time.Millisecond, func() { tm.Rearm(time.Millisecond) })
	in.Advance(time.Millisecond)
	if n := testing.AllocsPerRun(100, func() { in.Advance(time.Millisecond) }); n != 0 {
		t.Fatalf("a re-arming expiry allocates %v times, want 0", n)
	}
}
