package gameservertest

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// TestSettleWaitsForTheTaskThatClosedItsQueue: a task that closes its own
// queue (a corpse's decay closing the corpse's queue in its despawn cleanup)
// still has work left to run after the close. Settle must not return before
// that task has finished, although its queue refuses the barrier post.
func TestSettleWaitsForTheTaskThatClosedItsQueue(t *testing.T) {
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
	q := qs.NewQueue("closing")

	closed, release := make(chan struct{}), make(chan struct{})
	var finished atomic.Bool
	if !q.Post(func() {
		q.Close()
		close(closed)
		<-release
		finished.Store(true)
	}) {
		t.Fatal("open queue refused the task")
	}
	<-closed

	settled := make(chan bool, 1)
	go func() {
		err := qs.settle()
		if err != nil {
			t.Errorf("settle: %v", err)
		}
		settled <- finished.Load()
	}()
	// Give a settle that does not wait for the closing task every chance to
	// return before the task can finish. A settle that waits is blocked
	// until release, so this cannot make it fail.
	for range 10_000 {
		runtime.Gosched()
	}
	close(release)
	if !<-settled {
		t.Fatal("settle returned while the task that closed its queue was still running")
	}
}
