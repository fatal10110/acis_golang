package sim

import (
	"testing"
	"time"

	"github.com/rs/zerolog"
)

var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func TestInlineRunsAllQueuesInGlobalPostOrder(t *testing.T) {
	in := NewInline(epoch, zerolog.Nop())
	a, b := in.NewQueue("a"), in.NewQueue("b")
	var order []string
	a.Post(func() {
		AssertOwner(a)
		order = append(order, "a1")
		b.Post(func() { order = append(order, "b2") })
	})
	b.Post(func() { order = append(order, "b1") })
	a.Post(func() { order = append(order, "a2") })

	if len(order) != 0 {
		t.Fatalf("Post ran a task inline: %v", order)
	}
	in.Run()

	want := []string{"a1", "b1", "a2", "b2"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

func TestInlineAdvanceFiresExactlyTheDueTimers(t *testing.T) {
	in := NewInline(epoch, zerolog.Nop())
	q := in.NewQueue("q")
	type fire struct {
		name string
		at   time.Duration
	}
	var fired []fire
	record := func(name string) func() {
		return func() { fired = append(fired, fire{name, in.Now().Sub(epoch)}) }
	}
	ms := time.Millisecond

	q.After(10*ms, record("t10"))
	q.After(30*ms, record("t30"))
	cancelled := q.After(20*ms, record("cancelled"))
	q.Every(15*ms, record("tick"))
	q.After(5*ms, func() {
		record("t5")()
		cancelled.Stop()                      // runs before the 10 ms timer fires
		q.After(3*ms, record("t8 (from t5)")) // armed at 5 ms
	})

	in.Advance(20 * ms)
	want := []fire{{"t5", 5 * ms}, {"t8 (from t5)", 8 * ms}, {"t10", 10 * ms}, {"tick", 15 * ms}}
	assertFires(t, fired, want)
	if got := in.Now().Sub(epoch); got != 20*ms {
		t.Fatalf("clock at %v after Advance, want 20ms", got)
	}

	fired = nil
	in.Advance(10 * ms)
	// Same deadline: t30 was armed before the tick re-armed itself at 15 ms.
	assertFires(t, fired, []fire{{"t30", 30 * ms}, {"tick", 30 * ms}})

	fired = nil
	q.Close()
	in.Advance(time.Second)
	assertFires(t, fired, nil)
}

func TestInlineTimerStopCancelsBeforeFire(t *testing.T) {
	in := NewInline(epoch, zerolog.Nop())
	q := in.NewQueue("q")
	var n int
	tm := q.After(time.Millisecond, func() { n++ })
	if !tm.Stop() {
		t.Fatal("Stop on an armed timer reported no cancel")
	}
	in.Advance(time.Second)
	if n != 0 {
		t.Fatal("stopped timer fired")
	}

	tm = q.After(time.Millisecond, func() { n++ })
	in.Advance(time.Millisecond)
	in.Advance(time.Second)
	if n != 1 || tm.Stop() {
		t.Fatalf("fired %d times, Stop after expiry must report false", n)
	}
}

func assertFires[T comparable](t *testing.T, got, want []T) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("fired %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("fired %v, want %v", got, want)
		}
	}
}
