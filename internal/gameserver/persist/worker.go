// Package persist runs database writes off the goroutines that mutate game
// state. Jobs for one owner run one at a time in the order they were
// enqueued, so a later save for an owner can never land before an earlier
// one.
package persist

import (
	"context"
	"sync"

	"github.com/rs/zerolog"
)

// Lanes is how many goroutines run jobs. Every owner id maps to one lane.
const Lanes = 4

// Worker runs persistence jobs on a fixed set of lanes. Each lane is one
// goroutine draining an unbounded FIFO, so Enqueue never blocks the caller.
//
// A nil *Worker runs every job inline on the caller, for code built without
// a worker.
type Worker struct {
	log   zerolog.Logger
	lanes [Lanes]lane
	done  sync.WaitGroup
}

// lane is one FIFO of jobs. mu guards jobs and closed; cond wakes the lane
// goroutine when either changes.
type lane struct {
	mu     sync.Mutex
	cond   sync.Cond
	jobs   []func()
	closed bool
}

// New starts a worker's lane goroutines. Close stops them.
func New(log zerolog.Logger) *Worker {
	w := &Worker{log: log}
	w.done.Add(Lanes)
	for i := range w.lanes {
		l := &w.lanes[i]
		l.cond.L = &l.mu
		go w.run(l)
	}
	return w
}

// Enqueue appends job to ownerID's lane. It reports false, without running
// job, once the worker is closed.
func (w *Worker) Enqueue(ownerID int32, job func()) bool {
	if w == nil {
		job()
		return true
	}
	if !w.lane(ownerID).push(job) {
		w.log.Warn().Int32("owner_id", ownerID).Msg("persist: worker closed, job refused")
		return false
	}
	return true
}

// Flush waits until every job enqueued before the call on the lanes of
// ownerIDs has run, or ctx ends. With no ownerIDs it waits on every lane.
func (w *Worker) Flush(ctx context.Context, ownerIDs ...int32) error {
	if w == nil {
		return nil
	}
	var lanes [Lanes]bool
	for _, id := range ownerIDs {
		lanes[laneIndex(id)] = true
	}
	var pending sync.WaitGroup
	for i := range w.lanes {
		if len(ownerIDs) > 0 && !lanes[i] {
			continue
		}
		pending.Add(1)
		// A closed lane runs every job it accepted before exiting, and Close
		// waits for that, so nothing is left to wait for.
		if !w.lanes[i].push(pending.Done) {
			pending.Done()
		}
	}
	return wait(ctx, &pending)
}

// Close refuses new jobs, runs every job already enqueued, and waits for the
// lane goroutines to exit, or for ctx to end.
func (w *Worker) Close(ctx context.Context) error {
	if w == nil {
		return nil
	}
	for i := range w.lanes {
		l := &w.lanes[i]
		l.mu.Lock()
		l.closed = true
		l.mu.Unlock()
		l.cond.Signal()
	}
	return wait(ctx, &w.done)
}

func (w *Worker) lane(ownerID int32) *lane {
	return &w.lanes[laneIndex(ownerID)]
}

func laneIndex(ownerID int32) uint32 {
	return uint32(ownerID) % Lanes
}

func (l *lane) push(job func()) bool {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return false
	}
	l.jobs = append(l.jobs, job)
	l.mu.Unlock()
	l.cond.Signal()
	return true
}

func (w *Worker) run(l *lane) {
	defer w.done.Done()
	for {
		l.mu.Lock()
		for len(l.jobs) == 0 && !l.closed {
			l.cond.Wait()
		}
		jobs := l.jobs
		l.jobs = nil
		l.mu.Unlock()
		if len(jobs) == 0 {
			return
		}
		for _, job := range jobs {
			w.runJob(job)
		}
	}
}

func (w *Worker) runJob(job func()) {
	defer func() {
		if r := recover(); r != nil {
			w.log.Error().Interface("panic", r).Msg("persist: recovered panic in job")
		}
	}()
	job()
}

func wait(ctx context.Context, wg *sync.WaitGroup) error {
	done := make(chan struct{})
	// Outlives an expired ctx only until wg drains; jobs bound their own I/O.
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
