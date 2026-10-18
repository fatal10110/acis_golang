package sim

import (
	"testing"
	"time"
)

// A timer AfterPastClose armed outlives Close and runs as the queue's task;
// one armed after Close runs too, and Stop still cancels one. Close keeps
// cancelling every other timer and refusing posts.
func TestAfterPastCloseOutlivesClose(t *testing.T) {
	in := NewInline(epoch)
	q := in.NewQueue("q")
	ms := time.Millisecond
	var fired []string
	record := func(name string) func() {
		return func() {
			AssertOwner(q)
			fired = append(fired, name)
		}
	}

	q.AfterPastClose(10*ms, func() {
		record("past10")()
		q.AfterPastClose(5*ms, record("past15 (armed closed)"))
		q.After(1*ms, record("plain armed closed"))
	})
	stopped := q.AfterPastClose(20*ms, record("stopped"))
	q.After(10*ms, record("plain"))
	q.Every(5*ms, record("tick"))
	q.Close()
	if q.Post(record("post")) {
		t.Fatal("Post accepted after Close")
	}
	stopped.Stop()

	in.Advance(time.Second)
	want := []string{"past10", "past15 (armed closed)"}
	if len(fired) != len(want) {
		t.Fatalf("fired = %q, want %q", fired, want)
	}
	for i := range want {
		if fired[i] != want[i] {
			t.Fatalf("fired = %q, want %q", fired, want)
		}
	}
}
