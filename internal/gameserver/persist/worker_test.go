package persist

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestEnqueueRunsOneOwnersJobsInOrder(t *testing.T) {
	w := New(zerolog.Nop())
	defer w.Close(context.Background())

	const owners, perOwner = 16, 500
	var mu sync.Mutex
	got := make(map[int32][]int)
	var senders sync.WaitGroup
	for owner := range int32(owners) {
		senders.Go(func() {
			for i := range perOwner {
				w.Enqueue(owner, func() {
					mu.Lock()
					got[owner] = append(got[owner], i)
					mu.Unlock()
				})
			}
		})
	}
	senders.Wait()
	if err := w.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}

	for owner := range int32(owners) {
		seq := got[owner]
		if len(seq) != perOwner {
			t.Fatalf("owner %d ran %d jobs, want %d", owner, len(seq), perOwner)
		}
		for i, v := range seq {
			if v != i {
				t.Fatalf("owner %d job %d ran as %d", owner, i, v)
			}
		}
	}
}

// A job blocked on one lane must not delay another lane, and must not let a
// later job for the same owner overtake it.
func TestBlockedJobHoldsOnlyItsOwnersLane(t *testing.T) {
	w := New(zerolog.Nop())
	defer w.Close(context.Background())

	release := make(chan struct{})
	var order []string
	var mu sync.Mutex
	record := func(s string) {
		mu.Lock()
		order = append(order, s)
		mu.Unlock()
	}
	w.Enqueue(1, func() { <-release; record("first") })
	w.Enqueue(1, func() { record("second") })

	other := make(chan struct{})
	w.Enqueue(2, func() { close(other) })
	select {
	case <-other:
	case <-time.After(2 * time.Second):
		t.Fatal("owner 2's job waited on owner 1's blocked lane")
	}

	close(release)
	if err := w.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Fatalf("owner 1 order = %v, want [first second]", order)
	}
}

func TestFlushHonoursContext(t *testing.T) {
	w := New(zerolog.Nop())
	release := make(chan struct{})
	w.Enqueue(3, func() { <-release })

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := w.Flush(ctx); err == nil {
		t.Fatal("Flush returned nil while a job was still blocked")
	}
	close(release)
	if err := w.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCloseRunsQueuedJobsThenRefuses(t *testing.T) {
	w := New(zerolog.Nop())
	release := make(chan struct{})
	ran := 0
	w.Enqueue(5, func() { <-release })
	for range 10 {
		w.Enqueue(5, func() { ran++ })
	}

	closed := make(chan error)
	go func() { closed <- w.Close(context.Background()) }()
	close(release)
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	if ran != 10 {
		t.Fatalf("Close ran %d queued jobs, want 10", ran)
	}
	if w.Enqueue(5, func() { t.Error("job ran after Close") }) {
		t.Fatal("Enqueue after Close reported true")
	}
	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush after Close = %v, want nil", err)
	}
}

func TestPanickingJobDoesNotStopItsLane(t *testing.T) {
	w := New(zerolog.Nop())
	defer w.Close(context.Background())

	ran := false
	w.Enqueue(7, func() { panic("boom") })
	w.Enqueue(7, func() { ran = true })
	if err := w.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Fatal("job after a panicking job did not run")
	}
}

func TestNilWorkerRunsInline(t *testing.T) {
	var w *Worker
	ran := false
	if !w.Enqueue(1, func() { ran = true }) || !ran {
		t.Fatal("nil worker did not run the job inline")
	}
	if err := w.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
}
