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

// lane is one FIFO of jobs. mu guards jobs, closed and the owed set; cond
// wakes the lane goroutine when jobs or closed change, and settled wakes a
// Flush waiting for owed work.
type lane struct {
	mu      sync.Mutex
	cond    sync.Cond
	settled sync.Cond
	jobs    []func()
	closed  bool
	// owed is the work accepted on this lane that outlives the job that
	// started it, keyed by the order it was accepted in. A job that hands
	// itself back to the lane has returned but is not finished, so a Flush
	// that only waited for the jobs it can see would report a lane clear
	// while such work is still queued behind its own marker.
	owed     map[uint64]struct{}
	nextOwed uint64
}

// New starts a worker's lane goroutines. Close stops them.
func New(log zerolog.Logger) *Worker {
	w := &Worker{log: log}
	w.done.Add(Lanes)
	for i := range w.lanes {
		l := &w.lanes[i]
		l.cond.L = &l.mu
		l.settled.L = &l.mu
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

// Owe records work accepted on ownerID's lane that outlives the job that
// started it: a job that gives the lane back rather than waiting on it, and
// comes back later to finish. Flush waits for that work too, so a caller that
// waits for a lane still learns when nothing it queued is outstanding. Every
// Owe has to be matched by one Settle, whichever way the work ends.
func (w *Worker) Owe(ownerID int32) Owed {
	if w == nil {
		return Owed{}
	}
	l := w.lane(ownerID)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.nextOwed++
	if l.owed == nil {
		l.owed = make(map[uint64]struct{})
	}
	l.owed[l.nextOwed] = struct{}{}
	return Owed{lane: l, seq: l.nextOwed}
}

// Owed is one piece of work a lane is still waiting on. The zero value
// settles nothing, for code built without a worker.
type Owed struct {
	lane *lane
	seq  uint64
}

// Settle reports the work as finished, however it ended — written, dropped as
// superseded, or cancelled.
func (o Owed) Settle() {
	if o.lane == nil {
		return
	}
	o.lane.mu.Lock()
	defer o.lane.mu.Unlock()
	delete(o.lane.owed, o.seq)
	o.lane.settled.Broadcast()
}

// Flush waits until every job enqueued before the call on the lanes of
// ownerIDs has run and every piece of work those lanes already owed (Owe) has
// settled, or ctx ends. With no ownerIDs it waits on every lane.
func (w *Worker) Flush(ctx context.Context, ownerIDs ...int32) error {
	if w == nil {
		return nil
	}
	var lanes [Lanes]bool
	for _, id := range ownerIDs {
		lanes[LaneIndex(id)] = true
	}
	var pending sync.WaitGroup
	for i := range w.lanes {
		if len(ownerIDs) > 0 && !lanes[i] {
			continue
		}
		// Taken before the marker: work owed after this point belongs to a
		// later caller, and waiting for it could hold this one open for as
		// long as the lane keeps busy.
		high := w.lanes[i].owedHigh()
		pending.Add(2)
		// A closed lane still runs every job it accepted before its
		// goroutine exits, and Close marks lanes closed before that drain
		// finishes, so a refused marker waits for the drain instead.
		if !w.lanes[i].push(pending.Done) {
			// ponytail: waits for every lane's drain, not just this one;
			// only reachable while Close is running, per-lane exit signal if
			// that window ever matters.
			go func() {
				w.done.Wait()
				pending.Done()
			}()
		}
		go w.lanes[i].waitSettled(high, pending.Done)
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

// owedHigh reports the last piece of work this lane has accepted.
func (l *lane) owedHigh() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.nextOwed
}

// waitSettled calls done once nothing the lane owed up to high is left.
func (l *lane) waitSettled(high uint64, done func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for l.owedBefore(high) {
		l.settled.Wait()
	}
	done()
}

// owedBefore reports whether any work accepted up to high is still owed. It
// runs under l.mu.
func (l *lane) owedBefore(high uint64) bool {
	for seq := range l.owed {
		if seq <= high {
			return true
		}
	}
	return false
}

func (w *Worker) lane(ownerID int32) *lane {
	return &w.lanes[LaneIndex(ownerID)]
}

// LaneIndex is the lane ownerID's jobs run on.
func LaneIndex(ownerID int32) uint32 {
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
