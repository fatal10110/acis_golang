package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// TestStopAndWaitCancelsAContextTick runs a StartContext tick that would
// block for an hour unless its context ends. StopAndWait must cancel that
// context and return once the tick sees it, instead of waiting out the tick.
func TestStopAndWaitCancelsAContextTick(t *testing.T) {
	entered := make(chan struct{})
	ended := make(chan error, 1)
	ticker := StartContext(time.Millisecond, func(ctx context.Context) {
		select {
		case <-entered:
			return
		default:
		}
		close(entered)
		select {
		case <-ctx.Done():
			ended <- ctx.Err()
		case <-time.After(time.Hour):
			ended <- nil
		}
	}, zerolog.Nop())

	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("tick did not start")
	}

	stopped := make(chan struct{})
	go func() {
		ticker.StopAndWait()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("StopAndWait waited out the tick instead of canceling it")
	}
	if err := <-ended; !errors.Is(err, context.Canceled) {
		t.Fatalf("tick context ended with %v, want context.Canceled", err)
	}
}

// TestContextTickRunsUncanceledUntilStop pins the other half: a running
// ticker hands its ticks a live context, so a tick is not cut short merely by
// starting.
func TestContextTickRunsUncanceledUntilStop(t *testing.T) {
	seen := make(chan error, 1)
	ticker := StartContext(time.Millisecond, func(ctx context.Context) {
		select {
		case seen <- ctx.Err():
		default:
		}
	}, zerolog.Nop())
	defer ticker.StopAndWait()

	select {
	case err := <-seen:
		if err != nil {
			t.Fatalf("tick context already ended with %v before Stop", err)
		}
	case <-time.After(time.Second):
		t.Fatal("tick did not run")
	}
}
