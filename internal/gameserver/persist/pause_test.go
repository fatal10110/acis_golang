package persist

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// Pause orders a write across every lane: jobs queued before it on any lane
// run before fn, and jobs queued after it on any lane run after fn, even
// when one lane is still busy when Pause is taken.
func TestPauseRunsBetweenEarlierAndLaterJobsOnEveryLane(t *testing.T) {
	w := New(zerolog.Nop())
	defer w.Close(context.Background())

	var mu sync.Mutex
	var order []string
	record := func(s string) {
		mu.Lock()
		order = append(order, s)
		mu.Unlock()
	}
	busy := make(chan struct{})
	w.Enqueue(1, func() { <-busy })
	for owner := range int32(Lanes) {
		w.Enqueue(owner, func() { record("before") })
	}

	p := w.Pause()
	for owner := range int32(Lanes) {
		w.Enqueue(owner, func() { record("after") })
	}
	done := make(chan error, 1)
	go func() {
		done <- p.Run(context.Background(), func() error {
			record("fn")
			return nil
		})
	}()

	select {
	case <-done:
		t.Fatal("Run returned while lane 1 still held an earlier job")
	case <-time.After(50 * time.Millisecond):
	}
	close(busy)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	want := 2*Lanes + 1
	if len(order) != want {
		t.Fatalf("ran %v, want %d entries", order, want)
	}
	for i, s := range order {
		switch {
		case i < Lanes && s != "before", i == Lanes && s != "fn", i > Lanes && s != "after":
			t.Fatalf("ran %v, want every before, then fn, then every after", order)
		}
	}
}

// A cancelled Run skips fn and still releases the lanes.
func TestPauseRunCancelledReleasesLanes(t *testing.T) {
	w := New(zerolog.Nop())
	defer w.Close(context.Background())

	busy := make(chan struct{})
	w.Enqueue(2, func() { <-busy })
	p := w.Pause()
	ran := make(chan struct{})
	w.Enqueue(2, func() { close(ran) })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	if err := p.Run(ctx, func() error { called = true; return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v, want context.Canceled", err)
	}
	if called {
		t.Fatal("cancelled Run called fn")
	}
	close(busy)
	select {
	case <-ran:
	case <-time.After(5 * time.Second):
		t.Fatal("a job queued behind a cancelled pause never ran")
	}
}

// A nil worker runs fn at once.
func TestPauseNilWorkerRunsFn(t *testing.T) {
	var w *Worker
	called := false
	if err := w.Pause().Run(context.Background(), func() error { called = true; return nil }); err != nil || !called {
		t.Fatalf("Run = %v, called %v; want nil and true", err, called)
	}
}
