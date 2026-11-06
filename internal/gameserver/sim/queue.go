// Package sim runs actor work on per-owner FIFO queues. A Queue's tasks run
// one at a time, in post order, on a Pool worker in production or on the
// goroutine driving an Inline loop in tests. State owned by a queue is
// touched only from its tasks; AssertOwner enforces that.
package sim

import (
	"strconv"
	"sync"
	"time"
)

// Clock reports the current time. Inline's clock moves only on Advance.
type Clock interface {
	Now() time.Time
}

// executor is what a Queue runs on: a Pool or an Inline loop. Now is the
// clock its timers run on.
type executor interface {
	Clock
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
	id string
	// serial, when non-zero, follows id in the queue's name (see Serial).
	serial int32
	exec   executor

	// draining is held while q's tasks run; AssertOwner probes it.
	draining sync.Mutex
	// running is the Pool worker slot draining q, nil otherwise. It is
	// guarded by draining.
	running *slot

	mu        sync.Mutex
	tasks     []func() // pending tasks; Pool only, Inline keeps one global FIFO
	scheduled bool     // Pool only: q is in the run queue or being drained
	backlog   bool     // Pool only: pending tasks crossed highWater and have not dropped back below it
	closed    bool
	accepted  uint64 // tasks and timer expiries the executor took from q, ever
	// timers heads the list of armed timers and tickers, linked through
	// Timer.prev and Timer.succ; Close walks it to cancel them.
	timers *Timer
}

// Post appends fn to the queue. It never runs fn inline and never waits for
// other tasks. Once the queue is closed or its pool is stopping it drops fn
// and returns false; a task Post accepted always runs.
func (q *Queue) Post(fn func()) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return !q.closed && q.enqueue(fn)
}

// enqueue hands fn to the executor and counts it when taken. Callers hold q.mu.
func (q *Queue) enqueue(fn func()) bool {
	if !q.exec.enqueue(q, fn) {
		return false
	}
	q.accepted++
	return true
}

// Accepted counts the tasks and timer expiries q has taken since it was
// created. A test harness compares it across a pass of barrier posts to tell
// whether any task posted work meanwhile.
func (q *Queue) Accepted() uint64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.accepted
}

// Now reads the clock q's timers run on: the wall clock on a Pool, the
// virtual clock on an Inline loop. Deadlines compared against q's timers
// must be taken from it.
func (q *Queue) Now() time.Time { return q.exec.Now() }

// Close cancels every armed timer and ticker, except those AfterPastClose
// armed, and refuses later posts. Tasks already accepted still run. Safe to
// call more than once.
func (q *Queue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
	for t := q.timers; t != nil; {
		succ := t.succ
		if !t.pastClose {
			t.done = true
			t.ct.Stop()
			q.unlink(t)
		}
		t = succ
	}
}

// link adds the armed t to q's timer list. Callers hold q.mu.
func (q *Queue) link(t *Timer) {
	t.prev, t.succ = nil, q.timers
	if q.timers != nil {
		q.timers.prev = t
	}
	q.timers = t
}

// unlink removes t from q's timer list; a t not on it is left alone.
// Callers hold q.mu.
func (q *Queue) unlink(t *Timer) {
	if t.prev == nil && q.timers != t {
		return
	}
	if t.prev != nil {
		t.prev.succ = t.succ
	} else {
		q.timers = t.succ
	}
	if t.succ != nil {
		t.succ.prev = t.prev
	}
	t.prev, t.succ = nil, nil
}

// After posts fn to q once d has elapsed, unless the timer is stopped or q
// is closed before fn starts.
func (q *Queue) After(d time.Duration, fn func()) *Timer {
	return q.arm(d, 0, fn, false)
}

// AfterPastClose is After for a step that must still run when q closes
// before it comes due: Close leaves it armed, and it arms on a closed queue
// too. Its expiry runs as one of q's tasks, behind the ones q accepted
// before it, unless the timer is stopped or q's pool is stopping.
func (q *Queue) AfterPastClose(d time.Duration, fn func()) *Timer {
	return q.arm(d, 0, fn, true)
}

// Every posts fn to q every d until the ticker is stopped or q is closed.
// Ticks keep a fixed rate: deadlines stay on the grid of the first one
// however late an expiry is handled. As with time.Ticker, a tick that comes
// due while the previous one is still waiting in the queue is dropped, and
// deadlines missed entirely are skipped.
func (q *Queue) Every(d time.Duration, fn func()) *Ticker {
	if d <= 0 {
		panic("sim: non-positive interval for Queue.Every")
	}
	return (*Ticker)(q.arm(d, d, fn, false))
}

func (q *Queue) arm(d, period time.Duration, fn func(), pastClose bool) *Timer {
	t := &Timer{q: q, fn: fn, period: period, pastClose: pastClose}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed && !pastClose {
		t.done = true
		return t
	}
	// Arming under q.mu orders the ct write before fire, which takes q.mu.
	t.next = q.exec.Now().Add(d)
	t.ct = q.exec.afterFunc(d, t.fire)
	q.link(t)
	return t
}

// Timer is a callback armed by Queue.After. Every field is guarded by q.mu.
type Timer struct {
	q      *Queue
	fn     func()
	period time.Duration // 0 for a one-shot
	next   time.Time     // deadline of the armed expiry
	ct     clockTimer
	done   bool // stopped, cancelled by Close, or a one-shot that ran
	// ran marks a one-shot whose fn has started and that nothing stopped
	// since: Rearm may arm it again.
	ran bool
	// pastClose marks a timer AfterPastClose armed, which Close leaves.
	pastClose bool
	queued    bool   // an expiry is posted and has not run yet
	post      func() // run as a func value, made at the first expiry
	// prev and succ link t into q's timer list while it is armed.
	prev, succ *Timer
}

// Stop cancels the timer and reports whether this call kept fn from running.
// An expiry already posted but not yet run is cancelled too. A one-shot
// stopped after its fn ran can no longer be re-armed.
func (t *Timer) Stop() bool {
	q := t.q
	q.mu.Lock()
	defer q.mu.Unlock()
	if t.done {
		t.ran = false
		return false
	}
	t.done = true
	q.unlink(t)
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
		now := q.exec.Now()
		t.next = t.next.Add(t.period)
		if !t.next.After(now) {
			t.next = t.next.Add((now.Sub(t.next)/t.period + 1) * t.period)
		}
		t.ct.Reset(t.next.Sub(now))
	}
	if t.queued {
		return
	}
	if t.post == nil {
		t.post = t.run
	}
	if !q.enqueue(t.post) {
		t.done = true
		q.unlink(t)
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
		t.done, t.ran = true, true
		q.unlink(t)
	}
	q.mu.Unlock()
	q.attribute(t.fn)
	t.fn()
}

// Rearm arms again a one-shot whose fn has run, from inside fn or later, to
// run fn once more d from now. It reuses t and its clock timer, so it
// allocates nothing. It reports false and arms nothing when fn has not run,
// when t was stopped since, or when q is closed and t is not an
// AfterPastClose timer.
func (t *Timer) Rearm(d time.Duration) bool {
	q := t.q
	q.mu.Lock()
	defer q.mu.Unlock()
	if !t.ran || q.closed && !t.pastClose {
		return false
	}
	t.ran, t.done = false, false
	t.next = q.exec.Now().Add(d)
	t.ct.Reset(d)
	q.link(t)
	return true
}

// Ticker is a periodic callback armed by Queue.Every.
type Ticker Timer

// Stop halts future ticks, including one already posted but not yet run.
// Safe to call more than once.
func (k *Ticker) Stop() { (*Timer)(k).Stop() }

// Serial makes n part of q's name: its id, a dash, then n, built only
// when a log line or panic names q. Call it before q is shared.
func (q *Queue) Serial(n int32) *Queue {
	q.serial = n
	return q
}

// name is what logs and panics call q.
func (q *Queue) name() string {
	if q.serial == 0 {
		return q.id
	}
	return q.id + "-" + strconv.FormatInt(int64(q.serial), 10)
}

// AssertOwner panics unless the caller is running one of q's tasks. Every
// build catches state touched while q is idle; the simdebug build also
// catches a caller on another goroutine while q is busy.
func AssertOwner(q *Queue) {
	if q.draining.TryLock() {
		q.draining.Unlock()
		panic("sim: state of queue " + q.name() + " touched off its queue")
	}
	assertDrainer(q)
}

// Attribute names fn, in place of the task itself, as the work q's running
// task does in the pool's log lines for that task. A task that wraps the
// work it was posted for in a closure of its own calls it, so a slow-task
// report names that work. fn must be a function. Call it only from one of
// q's tasks; on an Inline loop, or under RunOwned, it names nothing.
func Attribute(q *Queue, fn any) {
	AssertOwner(q)
	q.attribute(fn)
}

// attribute is Attribute for a caller already known to own q.
func (q *Queue) attribute(fn any) {
	if s := q.running; s != nil {
		s.mu.Lock()
		s.task = fn
		s.mu.Unlock()
	}
}

// RunOwned runs fn on the calling goroutine as q's owner, once q's in-flight
// drain, if any, has returned; on a Pool that is the whole batch the drain
// took, not one task. It is for work that must still run after
// Post refused it: the queue is closed or its pool is stopping, but tasks it
// accepted earlier may still be draining. A task may also call it to act as
// the owner of another queue whose tasks never wait on the caller's. A task
// must not call it for its own queue.
func RunOwned(q *Queue, fn func()) { drainAs(q, fn) }

// drainAs runs fn as q's owner: q.draining is held and, under simdebug, the
// calling goroutine is recorded as q's drainer.
func drainAs(q *Queue, fn func()) {
	q.draining.Lock()
	outer := enterDrain(q)
	defer func() {
		exitDrain(outer)
		q.draining.Unlock()
	}()
	fn()
}
