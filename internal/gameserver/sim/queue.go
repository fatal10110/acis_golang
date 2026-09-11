// Package sim runs actor work on per-owner FIFO queues. A Queue's tasks run
// one at a time, in post order, on a Pool worker in production or on the
// goroutine driving an Inline loop in tests. State owned by a queue is
// touched only from its tasks; AssertOwner enforces that.
package sim

import (
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// drainSlice bounds how many tasks one drain runs from a queue before the
// queue goes to the back of the run queue, so one busy owner cannot starve
// the rest.
const drainSlice = 64

// slowTask is the task duration above which the watchdog logs the queue.
const slowTask = 50 * time.Millisecond

// Clock reports the current time. Inline's clock moves only on Advance.
type Clock interface {
	Now() time.Time
}

// SystemClock is the wall clock.
type SystemClock struct{}

// Now returns time.Now().
func (SystemClock) Now() time.Time { return time.Now() }

// executor is what a Queue runs on: a Pool or an Inline loop.
type executor interface {
	// enqueue accepts fn for q. It is called with q.mu held.
	enqueue(q *Queue, fn func()) bool
	afterFunc(d time.Duration, fn func()) clockTimer
}

// clockTimer is a runtime *time.Timer or an Inline virtual timer.
type clockTimer interface {
	Stop() bool
	Reset(d time.Duration) bool
}

// Queue is one owner's FIFO of tasks.
type Queue struct {
	id   string
	exec executor

	// draining is held while q's tasks run; AssertOwner probes it.
	draining sync.Mutex

	mu        sync.Mutex
	tasks     []func() // pending tasks; Pool only, Inline keeps one global FIFO
	scheduled bool     // Pool only: q is in the run queue or being drained
	closed    bool
	timers    map[*Timer]struct{} // armed timers and tickers, cancelled by Close
}

// Post appends fn to the queue. It never runs fn inline and never waits for
// other tasks. Once the queue is closed or its pool is stopping it drops fn
// and returns false; a task Post accepted always runs.
func (q *Queue) Post(fn func()) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return !q.closed && q.exec.enqueue(q, fn)
}

// Close cancels every armed timer and ticker and refuses later posts. Tasks
// already accepted still run. Safe to call more than once.
func (q *Queue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
	for t := range q.timers {
		t.done = true
		t.ct.Stop()
	}
	q.timers = nil
}

// After posts fn to q once d has elapsed, unless the timer is stopped or q
// is closed before fn starts.
func (q *Queue) After(d time.Duration, fn func()) *Timer {
	return q.arm(d, 0, fn)
}

// Every posts fn to q every d until the ticker is stopped or q is closed. A
// tick that comes due while the previous one is still waiting in the queue
// is dropped, as with time.Ticker.
func (q *Queue) Every(d time.Duration, fn func()) *Ticker {
	if d <= 0 {
		panic("sim: non-positive interval for Queue.Every")
	}
	return (*Ticker)(q.arm(d, d, fn))
}

func (q *Queue) arm(d, period time.Duration, fn func()) *Timer {
	t := &Timer{q: q, fn: fn, period: period}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		t.done = true
		return t
	}
	// Arming under q.mu orders the ct write before fire, which takes q.mu.
	t.ct = q.exec.afterFunc(d, t.fire)
	if q.timers == nil {
		q.timers = make(map[*Timer]struct{})
	}
	q.timers[t] = struct{}{}
	return t
}

// Timer is a callback armed by Queue.After. Every field is guarded by q.mu.
type Timer struct {
	q      *Queue
	fn     func()
	period time.Duration // 0 for a one-shot
	ct     clockTimer
	done   bool // stopped, cancelled by Close, or a one-shot that ran
	queued bool // an expiry is posted and has not run yet
}

// Stop cancels the timer and reports whether this call kept fn from running.
// An expiry already posted but not yet run is cancelled too.
func (t *Timer) Stop() bool {
	q := t.q
	q.mu.Lock()
	defer q.mu.Unlock()
	if t.done {
		return false
	}
	t.done = true
	delete(q.timers, t)
	t.ct.Stop()
	return true
}

// fire runs on the clock's goroutine when the timer comes due.
func (t *Timer) fire() {
	q := t.q
	q.mu.Lock()
	defer q.mu.Unlock()
	if t.done {
		return
	}
	if t.period > 0 {
		// ponytail: re-armed from the expiry, so each tick drifts by the
		// callback latency (µs); anchor to the first deadline if it matters.
		t.ct.Reset(t.period)
	}
	if t.queued {
		return
	}
	if !q.exec.enqueue(q, t.run) {
		t.done = true
		delete(q.timers, t)
		t.ct.Stop()
		return
	}
	t.queued = true
}

// run is the posted expiry; it runs on q.
func (t *Timer) run() {
	q := t.q
	q.mu.Lock()
	if t.done {
		q.mu.Unlock()
		return
	}
	t.queued = false
	if t.period == 0 {
		t.done = true
		delete(q.timers, t)
	}
	q.mu.Unlock()
	t.fn()
}

// Ticker is a periodic callback armed by Queue.Every.
type Ticker Timer

// Stop halts future ticks, including one already posted but not yet run.
// Safe to call more than once.
func (k *Ticker) Stop() { (*Timer)(k).Stop() }

// AssertOwner panics unless the caller is running one of q's tasks. Every
// build catches state touched while q is idle; the simdebug build also
// catches a caller on another goroutine while q is busy.
func AssertOwner(q *Queue) {
	if q.draining.TryLock() {
		q.draining.Unlock()
		panic("sim: state of queue " + q.id + " touched off its queue")
	}
	assertDrainer(q)
}

// runTask runs fn for queue, containing a panic and logging a slow task.
func runTask(log zerolog.Logger, queue string, fn func()) {
	start := time.Now()
	defer func() {
		if r := recover(); r != nil {
			log.Error().Str("queue", queue).Interface("panic", r).Msg("sim: recovered panic in queued task")
		}
		if d := time.Since(start); d > slowTask {
			log.Warn().Str("queue", queue).Dur("elapsed", d).Msg("sim: slow task")
		}
	}()
	fn()
}

// drainAs runs fn as q's owner: q.draining is held and, under simdebug, the
// calling goroutine is recorded as q's drainer.
func drainAs(q *Queue, fn func()) {
	q.draining.Lock()
	enterDrain(q)
	defer func() {
		exitDrain()
		q.draining.Unlock()
	}()
	fn()
}
