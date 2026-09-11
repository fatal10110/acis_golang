package sim

import (
	"bytes"
	"context"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func startPool(t *testing.T, workers int, log zerolog.Logger) *Pool {
	t.Helper()
	p := NewPool(workers, log)
	p.Start(context.Background())
	t.Cleanup(func() { stopPool(t, p) })
	return p
}

func stopPool(t *testing.T, p *Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := p.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestPoolKeepsPerPosterFIFOUnderConcurrentPosts(t *testing.T) {
	const posters, perPoster = 8, 2000
	p := NewPool(4, zerolog.Nop())
	p.Start(context.Background())
	q := p.NewQueue("q")

	type step struct{ poster, seq int }
	var ran []step // queue-owned: no lock
	var wg sync.WaitGroup
	for g := range posters {
		wg.Go(func() {
			for i := range perPoster {
				if !q.Post(func() { ran = append(ran, step{g, i}) }) {
					t.Error("Post refused on an open queue")
				}
			}
		})
	}
	wg.Wait()
	stopPool(t, p)

	if len(ran) != posters*perPoster {
		t.Fatalf("ran %d tasks, want %d", len(ran), posters*perPoster)
	}
	next := make([]int, posters)
	for _, s := range ran {
		if s.seq != next[s.poster] {
			t.Fatalf("poster %d: ran seq %d, want %d", s.poster, s.seq, next[s.poster])
		}
		next[s.poster]++
	}
}

func TestPoolDrainsEachQueueOnOneWorkerAtATime(t *testing.T) {
	const queues, perQueue = 1000, 20
	p := NewPool(2, zerolog.Nop())
	p.Start(context.Background())

	qs := make([]*Queue, queues)
	busy := make([]atomic.Bool, queues)
	counts := make([]int, queues) // plain ints: -race reports a second drainer
	for i := range qs {
		qs[i] = p.NewQueue("q")
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for n := range perQueue {
				for i := range qs {
					qs[(i+n)%queues].Post(func() {
						j := (i + n) % queues
						AssertOwner(qs[j])
						if !busy[j].CompareAndSwap(false, true) {
							t.Errorf("queue %d drained by two workers", j)
						}
						counts[j]++
						busy[j].Store(false)
					})
				}
			}
		})
	}
	wg.Wait()
	stopPool(t, p)

	for i, c := range counts {
		if c != 4*perQueue {
			t.Fatalf("queue %d ran %d tasks, want %d", i, c, 4*perQueue)
		}
	}
}

func TestPoolDrainSliceBoundsHowLongABusyQueueDelaysAnother(t *testing.T) {
	p := startPool(t, 1, zerolog.Nop())
	blocker, busy, other := p.NewQueue("blocker"), p.NewQueue("busy"), p.NewQueue("other")

	release := make(chan struct{})
	blocker.Post(func() { <-release })
	busyRan := 0 // one worker: tasks never overlap
	for range 10_000 {
		busy.Post(func() { busyRan++ })
	}
	got := make(chan int)
	other.Post(func() { got <- busyRan })
	close(release)

	if n := <-got; n != drainSlice {
		t.Fatalf("other ran after %d busy tasks, want %d (one slice)", n, drainSlice)
	}
}

func TestPoolStopRunsAcceptedTasksAndRefusesLaterPosts(t *testing.T) {
	p := NewPool(2, zerolog.Nop())
	q := p.NewQueue("q")
	var ran atomic.Int32
	for range 500 {
		q.Post(func() { ran.Add(1) }) // accepted before Start
	}
	p.Start(context.Background())
	stopPool(t, p)

	if got := ran.Load(); got != 500 {
		t.Fatalf("ran %d accepted tasks, want 500", got)
	}
	if p.NewQueue("late").Post(func() { t.Error("ran after Stop") }) {
		t.Fatal("Post accepted after Stop")
	}
}

func TestQueuePostAfterCloseDrops(t *testing.T) {
	p := startPool(t, 1, zerolog.Nop())
	q := p.NewQueue("q")
	q.Close()
	if q.Post(func() { t.Error("ran after Close") }) {
		t.Fatal("Post accepted after Close")
	}
	q.Close() // idempotent
}

func TestTimerFiresOnceOnItsQueue(t *testing.T) {
	p := startPool(t, 2, zerolog.Nop())
	q := p.NewQueue("q")
	fired := make(chan struct{}, 2)
	tm := q.After(time.Millisecond, func() {
		AssertOwner(q)
		fired <- struct{}{}
	})
	<-fired
	if tm.Stop() {
		t.Fatal("Stop after expiry reported a cancel")
	}
	select {
	case <-fired:
		t.Fatal("timer fired twice")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestTimerStopAndCloseCancel(t *testing.T) {
	p := startPool(t, 1, zerolog.Nop())
	q := p.NewQueue("q")

	stopped := q.After(time.Millisecond, func() { t.Error("stopped timer fired") })
	if !stopped.Stop() {
		t.Fatal("Stop on an armed timer reported no cancel")
	}
	if stopped.Stop() {
		t.Fatal("second Stop reported a cancel")
	}

	closed := q.After(time.Millisecond, func() { t.Error("timer fired after Close") })
	ticker := q.Every(time.Millisecond, func() { t.Error("ticker fired after Close") })
	q.Close()
	if closed.Stop() {
		t.Fatal("Close left the timer armed")
	}
	ticker.Stop()
	if q.After(time.Millisecond, func() { t.Error("timer armed on a closed queue fired") }).Stop() {
		t.Fatal("timer armed on a closed queue reported a cancel")
	}
	time.Sleep(20 * time.Millisecond)
}

func TestTickerDropsTicksWhileOneIsQueued(t *testing.T) {
	p := startPool(t, 1, zerolog.Nop())
	q := p.NewQueue("q")
	release := make(chan struct{})
	q.Post(func() { <-release })

	var ticks int // queue-owned
	ticker := q.Every(2*time.Millisecond, func() { ticks++ })
	time.Sleep(30 * time.Millisecond) // ~15 periods while the queue is blocked
	got := make(chan int)
	q.Post(func() { got <- ticks }) // behind the one tick queued during the block
	close(release)

	if n := <-got; n != 1 {
		t.Fatalf("%d ticks ran for ~15 periods blocked behind one task, want 1", n)
	}
	ticker.Stop()
}

func TestPoolWatchdogAndPanicContainment(t *testing.T) {
	var buf lockedBuffer // the worker and the watchdog both log
	p := NewPool(1, zerolog.New(&buf))
	p.Start(context.Background())
	q := p.NewQueue("npc-42")

	q.Post(func() { panic("boom") })
	q.Post(func() { time.Sleep(slowTask + 10*time.Millisecond) })
	var after atomic.Bool
	q.Post(func() { after.Store(true) })
	stopPool(t, p)

	if !after.Load() {
		t.Fatal("task after a panic did not run")
	}
	out := buf.String()
	for _, want := range []string{`"panic":"boom"`, `sim: slow task`, `"queue":"npc-42"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("log missing %s:\n%s", want, out)
		}
	}
	if strings.Count(out, "slow task") != 1 {
		t.Fatalf("want exactly one slow-task line:\n%s", out)
	}
}

func TestPoolWatchdogReportsAStuckTaskWhileItRuns(t *testing.T) {
	var log lockedBuffer
	p := startPool(t, 1, zerolog.New(&log))
	q := p.NewQueue("npc-hung")
	release := make(chan struct{})
	defer close(release) // before the Cleanup that stops the pool
	q.Post(func() { <-release })

	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(log.String(), "sim: task still running") {
		if time.Now().After(deadline) {
			t.Fatalf("no log for a task blocked past the deadline:\n%s", log.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
	if out := log.String(); !strings.Contains(out, `"queue":"npc-hung"`) {
		t.Fatalf("stuck-task log does not name the queue:\n%s", out)
	}
	time.Sleep(3 * slowTask)
	if n := strings.Count(log.String(), "still running"); n != 1 {
		t.Fatalf("stuck task reported %d times, want once:\n%s", n, log.String())
	}
}

func TestPoolSurvivesATaskThatCallsGoexit(t *testing.T) {
	var log lockedBuffer
	p := startPool(t, 1, zerolog.New(&log)) // the only worker is the one that exits
	q := p.NewQueue("npc-goexit")

	var order []int // queue-owned
	done := make(chan []int)
	q.Post(func() { order = append(order, 0) })
	q.Post(func() { runtime.Goexit() })
	q.Post(func() { order = append(order, 2) }) // same batch, behind the Goexit
	q.Post(func() { order = append(order, 3); done <- order })

	select {
	case got := <-done:
		if want := []int{0, 2, 3}; !slices.Equal(got, want) {
			t.Fatalf("ran %v, want %v", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("tasks accepted behind a Goexit never ran")
	}
	other := make(chan struct{})
	p.NewQueue("other").Post(func() { close(other) })
	select {
	case <-other:
	case <-time.After(5 * time.Second):
		t.Fatal("pool lost its worker to a Goexit")
	}
	if out := log.String(); !strings.Contains(out, "runtime.Goexit") || !strings.Contains(out, `"queue":"npc-goexit"`) {
		t.Fatalf("Goexit not logged with its queue:\n%s", out)
	}
}

func TestPoolStopReleasesItsStartContext(t *testing.T) {
	ctx := &countingCtx{Context: t.Context()}
	for range 3 {
		p := NewPool(1, zerolog.Nop())
		p.Start(ctx)
		stopPool(t, p)
	}
	if n := ctx.live.Load(); n != 0 {
		t.Fatalf("%d stopped pools still registered on the live Start context", n)
	}
}

// countingCtx counts live context.AfterFunc registrations on it.
type countingCtx struct {
	context.Context
	live atomic.Int32
}

// Value hides the embedded cancelCtx so AfterFunc registers through c.
func (c *countingCtx) Value(any) any { return nil }

func (c *countingCtx) AfterFunc(func()) func() bool {
	c.live.Add(1)
	var stopped atomic.Bool
	return func() bool {
		if stopped.Swap(true) {
			return false
		}
		c.live.Add(-1)
		return true
	}
}

func TestTickerKeepsAFixedRate(t *testing.T) {
	m := &manualExec{at: epoch}
	q := &Queue{id: "q", exec: m}
	tick := (*Timer)(q.Every(10*time.Millisecond, func() {}))

	m.at = epoch.Add(13 * time.Millisecond) // expiry delivered 3 ms late
	tick.fire()
	m.at = epoch.Add(47 * time.Millisecond) // then blocked past the 20 and 30 ms deadlines
	tick.fire()

	// Deadlines stay on the 10 ms grid: 10, then 20, then 50 (30 and 40 dropped).
	want := []time.Duration{10 * time.Millisecond, 7 * time.Millisecond, 3 * time.Millisecond}
	if !slices.Equal(m.arms, want) {
		t.Fatalf("armed %v, want %v", m.arms, want)
	}
}

// manualExec is an executor whose clock and expiries the test drives; it
// records every delay the timer is armed with and runs nothing.
type manualExec struct {
	at   time.Time
	arms []time.Duration
}

func (m *manualExec) enqueue(*Queue, func()) bool { return true }
func (m *manualExec) Now() time.Time              { return m.at }
func (m *manualExec) afterFunc(d time.Duration, _ func()) clockTimer {
	m.arms = append(m.arms, d)
	return m
}
func (m *manualExec) Stop() bool { return true }
func (m *manualExec) Reset(d time.Duration) bool {
	m.arms = append(m.arms, d)
	return true
}

// lockedBuffer is a log sink the test can read while workers write to it.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func TestAssertOwner(t *testing.T) {
	p := startPool(t, 2, zerolog.Nop())
	q := p.NewQueue("q")

	if !panics(func() { AssertOwner(q) }) {
		t.Fatal("AssertOwner did not panic off-queue on an idle queue")
	}
	onQueue := make(chan bool)
	q.Post(func() { onQueue <- panics(func() { AssertOwner(q) }) })
	if <-onQueue {
		t.Fatal("AssertOwner panicked on its own queue")
	}
}

func panics(fn func()) (panicked bool) {
	defer func() { panicked = recover() != nil }()
	fn()
	return false
}
