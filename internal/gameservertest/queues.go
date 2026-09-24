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
// "inline" is sim.Inline, one global FIFO run by a single goroutine. In a
// suite that called DriveClock, "inline" is also the default and its clock
// moves only on Server.Advance; elsewhere it advances with the wall clock.
const SimExecutorEnv = "ACIS_SIM_EXECUTOR"

// inlinePumpInterval is how long the inline runner sleeps once it finds no
// work, bounding the delay a posted task waits before it runs.
const inlinePumpInterval = time.Millisecond

// drivenClock is set by DriveClock before any test runs and only read after.
var drivenClock bool

// DriveClock makes every Boot in the calling test binary run actor queues on
// sim.Inline with a clock that moves only when a test calls Server.Advance
// or Server.AdvanceUntil, unless SimExecutorEnv=pool asks for the real pool.
// Call it from TestMain before running the tests.
func DriveClock() { drivenClock = true }

// queues is the harness executor. It records every queue it creates so
// Settle can wait for the work already posted to them.
type queues struct {
	newQueue func(id string) *sim.Queue
	// inline and advance are set on a driven clock (DriveClock): the loop,
	// and a call that moves its clock by d on the runner and runs what
	// comes due.
	inline  *sim.Inline
	advance func(d time.Duration)

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

// startQueues starts the executor WithRealPool, SimExecutorEnv and
// DriveClock select, in that order of precedence, and stops it on cleanup.
// Register it before anything whose cleanup still posts to queues (the
// listener, whose connection handlers detach players on their queues).
func startQueues(tb testing.TB, log zerolog.Logger, realPool bool) *queues {
	tb.Helper()
	mode := os.Getenv(SimExecutorEnv)
	if realPool {
		mode = "pool"
	} else if mode == "" && drivenClock {
		mode = "inline"
	}
	switch mode {
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
		return startInline(tb)
	default:
		tb.Fatalf("%s=%q: want pool or inline", SimExecutorEnv, mode)
		return nil
	}
}

// startInline runs a sim.Inline loop on one runner goroutine, which is the
// only caller of Run and Advance. Connection goroutines post each frame's
// work and wait for it, so the runner keeps draining on its own. Under
// DriveClock the clock moves only through the returned advance; otherwise
// the runner advances it with the wall clock.
func startInline(tb testing.TB) *queues {
	start := time.Now()
	inline := sim.NewInline(start)
	type step struct {
		d    time.Duration
		done chan struct{}
	}
	steps := make(chan step)
	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		last := start
		for {
			if drivenClock {
				inline.Advance(0) // runs tasks and the timers already due
			} else {
				now := time.Now()
				inline.Advance(now.Sub(last))
				last = now
			}
			select {
			case s := <-steps:
				inline.Advance(s.d)
				close(s.done)
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
	q := &queues{newQueue: inline.NewQueue}
	if drivenClock {
		q.inline = inline
		q.advance = func(d time.Duration) {
			s := step{d: d, done: make(chan struct{})}
			steps <- s
			<-s.done
		}
	}
	return q
}

// OpenActorQueues counts the actor queues created so far that still accept a
// task. Every queue belongs to an actor that owns it until it detaches, so a
// queue left open with nobody behind it is a leak; posting a no-op is the
// only way to ask a queue whether it is still open.
func (s *Server) OpenActorQueues() int {
	s.queues.mu.Lock()
	all := append([]*sim.Queue(nil), s.queues.all...)
	s.queues.mu.Unlock()
	open := 0
	for _, queue := range all {
		if queue.Post(func() {}) {
			open++
		}
	}
	return open
}
