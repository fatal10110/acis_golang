package gameservertest

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// TestSettleWaitsForWorkPostedToAQueueItAlreadyPassed: a task on a later
// queue posts to an earlier queue after that queue's barrier ran (a monster's
// reaction posting the effect it casts onto the player's queue). One pass of
// barriers returns before that work runs; settle must wait for it.
func TestSettleWaitsForWorkPostedToAQueueItAlreadyPassed(t *testing.T) {
	pool := sim.NewPool(2, zerolog.Nop())
	pool.Start(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownDrainTimeout)
		defer cancel()
		if err := pool.Stop(ctx); err != nil {
			t.Errorf("stop sim pool: %v", err)
		}
	})
	qs := &queues{newQueue: pool.NewQueue}
	first, second := qs.NewQueue("first"), qs.NewQueue("second")

	// The second queue's task is still running when the first queue's barrier
	// passes; it then posts the first queue's task, which stays pending.
	gate, release := make(chan struct{}), make(chan struct{})
	var finished atomic.Bool
	if !second.Post(func() {
		<-gate
		first.Post(func() {
			<-release
			finished.Store(true)
		})
	}) {
		t.Fatal("open queue refused the task")
	}

	settled := make(chan bool, 1)
	go func() {
		if err := qs.settle(); err != nil {
			t.Errorf("settle: %v", err)
		}
		settled <- finished.Load()
	}()
	// Release the second task once settle's barrier waits behind it, which
	// is after the first queue's barrier ran.
	for second.Accepted() < 2 {
		runtime.Gosched()
	}
	close(gate)
	// A settle that stops after one pass returns while the first queue's
	// task is still blocked; one that waits is blocked until release, so
	// this loop cannot make it fail.
	for range 10_000 {
		runtime.Gosched()
	}
	close(release)
	if !<-settled {
		t.Fatal("settle returned before the work a later queue posted to an earlier one had run")
	}
}

// TestSettleRunsATimerArmedAlreadyDue: on the inline executor a timer armed
// with no delay (a cast that lands at once) fires when the runner next looks
// for work, which a barrier posted after the arming task does not wait for.
// Settle must not return before the timer's task has run.
func TestSettleRunsATimerArmedAlreadyDue(t *testing.T) {
	qs := startInline(t)
	q := qs.NewQueue("caster")
	for range 500 {
		var fired atomic.Bool
		if !q.Post(func() { q.After(0, func() { fired.Store(true) }) }) {
			t.Fatal("open queue refused the task")
		}
		if err := qs.settle(); err != nil {
			t.Fatalf("settle: %v", err)
		}
		if !fired.Load() {
			t.Fatal("settle returned before the timer armed already due had fired")
		}
	}
}
