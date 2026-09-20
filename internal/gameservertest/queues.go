package gameservertest

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// SimExecutorEnv selects what actor queues drain on for a test run: "pool"
// (the default) is the production sim.Pool, one worker per GOMAXPROCS;
// "inline" is sim.Inline, one global FIFO run by a single goroutine that
// advances its clock with the wall clock.
const SimExecutorEnv = "ACIS_SIM_EXECUTOR"

// inlinePumpInterval is how long the inline pump sleeps once it finds no
// work, bounding the delay a posted task waits before it runs.
const inlinePumpInterval = time.Millisecond

// queues is the harness executor. It records every queue it creates so
// Settle can wait for the work already posted to them.
type queues struct {
	newQueue func(id string) *sim.Queue

	mu  sync.Mutex
	all []*sim.Queue
}

func (q *queues) NewQueue(id string) *sim.Queue {
	queue := q.newQueue(id)
	q.mu.Lock()
	q.all = append(q.all, queue)
	q.mu.Unlock()
	return queue
}

// errUnsettled reports actor queues that kept a task pending past
// shutdownDrainTimeout.
var errUnsettled = errors.New("actor queues did not settle")

// settle waits until every task posted so far to a still-open queue has run.
func (q *queues) settle() error {
	q.mu.Lock()
	all := append([]*sim.Queue(nil), q.all...)
	q.mu.Unlock()
	deadline := time.After(shutdownDrainTimeout)
	for _, queue := range all {
		done := make(chan struct{})
		if !queue.Post(func() { close(done) }) {
			continue
		}
		select {
		case <-done:
		case <-deadline:
			return errUnsettled
		}
	}
	return nil
}

// startQueues starts the executor SimExecutorEnv selects and stops it on
// cleanup. Register it before anything whose cleanup still posts to queues
// (the listener, whose connection handlers detach players on their queues).
func startQueues(tb testing.TB, log zerolog.Logger) *queues {
	tb.Helper()
	switch mode := os.Getenv(SimExecutorEnv); mode {
	case "", "pool":
		pool := sim.NewPool(0, log)
		pool.Start(context.Background())
		tb.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), shutdownDrainTimeout)
			defer cancel()
			if err := pool.Stop(ctx); err != nil {
				tb.Errorf("stop sim pool: %v", err)
			}
		})
		return &queues{newQueue: pool.NewQueue}
	case "inline":
		start := time.Now()
		inline := sim.NewInline(start)
		stop, stopped := make(chan struct{}), make(chan struct{})
		go func() {
			defer close(stopped)
			last := start
			for {
				now := time.Now()
				inline.Advance(now.Sub(last))
				last = now
				select {
				case <-stop:
					inline.Run()
					return
				case <-time.After(inlinePumpInterval):
				}
			}
		}()
		tb.Cleanup(func() {
			close(stop)
			<-stopped
		})
		return &queues{newQueue: inline.NewQueue}
	default:
		tb.Fatalf("%s=%q: want pool or inline", SimExecutorEnv, mode)
		return nil
	}
}
