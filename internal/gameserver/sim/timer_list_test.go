package sim

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// linkedTimers walks q's armed-timer list, checking every back link and that
// no stopped, cancelled or ran timer is still on it.
func linkedTimers(t *testing.T, q *Queue) map[*Timer]bool {
	t.Helper()
	q.mu.Lock()
	defer q.mu.Unlock()
	got := map[*Timer]bool{}
	var prev *Timer
	for tm := q.timers; tm != nil; tm = tm.succ {
		if tm.prev != prev {
			t.Fatal("timer list back link broken")
		}
		if tm.done {
			t.Fatal("a done timer is still linked")
		}
		if got[tm] {
			t.Fatal("timer linked twice")
		}
		got[tm] = true
		prev = tm
	}
	return got
}

func wantLinked(t *testing.T, q *Queue, want ...*Timer) {
	t.Helper()
	got := linkedTimers(t, q)
	if len(got) != len(want) {
		t.Fatalf("%d timers linked, want %d", len(got), len(want))
	}
	for i, tm := range want {
		if !got[tm] {
			t.Fatalf("timer %d not linked", i)
		}
	}
}

// TestTimerStopUnlinksAtAnyPosition: Stop takes a timer off the list from
// its middle and either end, and a second Stop or a later Close leaves the
// list intact.
func TestTimerStopUnlinksAtAnyPosition(t *testing.T) {
	in := NewInline(epoch)
	q := in.NewQueue("q")
	tms := make([]*Timer, 5)
	for i := range tms {
		tms[i] = q.After(time.Hour, func() { t.Error("stopped timer fired") })
	}
	wantLinked(t, q, tms...)

	for _, i := range []int{2, 0, 4} {
		if !tms[i].Stop() {
			t.Fatalf("Stop of armed timer %d reported no cancel", i)
		}
	}
	wantLinked(t, q, tms[1], tms[3])
	for _, i := range []int{2, 0, 4} {
		if tms[i].Stop() {
			t.Fatalf("second Stop of timer %d reported a cancel", i)
		}
	}
	wantLinked(t, q, tms[1], tms[3])

	q.Close()
	q.Close()
	wantLinked(t, q)
	for _, i := range []int{1, 3} {
		if tms[i].Stop() {
			t.Fatalf("Stop after Close of timer %d reported a cancel", i)
		}
	}
	in.Advance(2 * time.Hour)
}

// TestCloseCancelsEveryTimerButPastClose: Close cancels armed one-shots,
// tickers and re-armed one-shots and leaves only the AfterPastClose timers,
// re-armed ones included; Close again keeps them, and one re-armed on the
// closed queue stays armed too.
func TestCloseCancelsEveryTimerButPastClose(t *testing.T) {
	in := NewInline(epoch)
	q := in.NewQueue("q")
	ms := time.Millisecond
	runs := map[string]int{}
	count := func(name string) func() { return func() { runs[name]++ } }

	plain := q.After(10*ms, count("plain"))
	tick := q.Every(5*ms, count("tick"))
	stopped := q.After(10*ms, count("stopped"))
	ranOnce := q.After(ms, count("ranOnce"))
	rearmed := q.After(ms, count("rearmed"))
	past := q.AfterPastClose(10*ms, count("past"))
	pastRearmed := q.AfterPastClose(ms, count("pastRearmed"))
	if !stopped.Stop() {
		t.Fatal("Stop of an armed timer reported no cancel")
	}
	in.Advance(ms)
	if !rearmed.Rearm(10*ms) || !pastRearmed.Rearm(10*ms) {
		t.Fatal("Rearm of a ran one-shot refused")
	}
	wantLinked(t, q, plain, (*Timer)(tick), rearmed, past, pastRearmed)

	q.Close()
	q.Close()
	wantLinked(t, q, past, pastRearmed)
	for name, tm := range map[string]*Timer{
		"plain": plain, "tick": (*Timer)(tick), "stopped": stopped, "ranOnce": ranOnce, "rearmed": rearmed,
	} {
		if tm.Stop() {
			t.Errorf("Stop of %s after Close reported a cancel", name)
		}
	}
	in.Advance(time.Second)
	wantLinked(t, q)

	if !pastRearmed.Rearm(10 * ms) {
		t.Fatal("Rearm of an AfterPastClose timer on a closed queue refused")
	}
	wantLinked(t, q, pastRearmed)
	q.Close()
	wantLinked(t, q, pastRearmed)
	in.Advance(time.Second)
	wantLinked(t, q)

	want := map[string]int{"ranOnce": 1, "rearmed": 1, "past": 1, "pastRearmed": 3}
	if len(runs) != len(want) {
		t.Fatalf("runs = %v, want %v", runs, want)
	}
	for name, n := range want {
		if runs[name] != n {
			t.Fatalf("runs = %v, want %v", runs, want)
		}
	}
}

// TestTimerFireOnStoppingPoolUnlinks: an expiry the stopping pool refuses
// retires its timer and takes it off the list.
func TestTimerFireOnStoppingPoolUnlinks(t *testing.T) {
	p := NewPool(1, zerolog.Nop())
	p.Start(context.Background())
	stopPool(t, p)
	q := p.NewQueue("q")
	refused := q.After(time.Hour, func() { t.Error("refused expiry ran") })
	other := q.After(time.Hour, func() { t.Error("stopped timer fired") })

	refused.fire()
	wantLinked(t, q, other)
	if refused.Stop() {
		t.Fatal("Stop of a refused timer reported a cancel")
	}
	if !other.Stop() {
		t.Fatal("Stop of an armed timer reported no cancel")
	}
	wantLinked(t, q)
}

// TestTimerArmAndStopAllocateOnlyTheTimer: arming n timers on a fresh queue
// and stopping them costs exactly n times what one does, so the armed set
// grows without allocations of its own.
func TestTimerArmAndStopAllocateOnlyTheTimer(t *testing.T) {
	if simdebugBuild {
		t.Skip("simdebug owner bookkeeping allocates per drain; the budget describes the production build")
	}
	in := NewInline(epoch)
	fn := func() {}
	const n = 64
	tms := make([]*Timer, n)
	armStop := func(k int) func() {
		return func() {
			q := in.NewQueue("q")
			for i := range k {
				tms[i] = q.After(time.Hour, fn)
			}
			for i := range k {
				tms[i].Stop()
			}
		}
	}
	base := testing.AllocsPerRun(20, armStop(0))
	one := testing.AllocsPerRun(20, armStop(1)) - base
	many := testing.AllocsPerRun(20, armStop(n)) - base
	if one <= 0 || many != n*one {
		t.Fatalf("arming and stopping %d timers allocates %v times, want %d x %v", n, many, n, one)
	}
}
