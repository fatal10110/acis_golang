package sim

import (
	"context"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
)

// drainSlice bounds how many tasks one drain runs from a queue before the
// queue goes to the back of the run queue, so one busy owner cannot starve
// the rest.
const drainSlice = 64

// slowTask is the task duration above which the watchdog logs the queue.
const slowTask = 50 * time.Millisecond

// Pool drains queues on a fixed set of worker goroutines. A queue is never
// drained by two workers at once.
type Pool struct {
	log    zerolog.Logger
	slots  []slot       // one per worker
	live   atomic.Int32 // workers not yet exited
	exited chan struct{}

	// ponytail: one run-queue lock shared by every worker; per-worker deques
	// with stealing if it shows up in profiles.
	mu      sync.Mutex
	wake    sync.Cond
	runq    []*Queue
	stopped atomic.Bool // written under mu
}

// slot is the task one worker is running, read by the watchdog.
//
// ponytail: two uncontended lock pairs per task; a seqlock over atomics if
// it shows up in profiles.
type slot struct {
	mu    sync.Mutex
	queue string
	start time.Time // zero while idle
	seq   uint64    // tasks started on this worker
}

// NewPool returns a pool of workers goroutines, GOMAXPROCS when workers is
// not positive. Posts are accepted before Start and run once it is called.
func NewPool(workers int, log zerolog.Logger) *Pool {
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	p := &Pool{log: log, slots: make([]slot, workers), exited: make(chan struct{})}
	p.wake.L = &p.mu
	return p
}

// NewQueue returns an open queue drained by p. id names it in logs.
func (p *Pool) NewQueue(id string) *Queue {
	return &Queue{id: id, exec: p}
}

// Start launches the workers and the watchdog. Cancelling ctx stops the pool
// as Stop does, without waiting.
func (p *Pool) Start(ctx context.Context) {
	idle := make(chan struct{})
	p.live.Store(int32(len(p.slots)))
	for i := range p.slots {
		go p.work(&p.slots[i], idle)
	}
	release := context.AfterFunc(ctx, p.shutdown)
	go func() {
		p.watch(idle)
		release() // a stopped pool must not stay reachable from a live ctx
		close(p.exited)
	}()
}

// Stop refuses further posts, lets the workers finish every task already
// accepted, and returns once they and the watchdog have exited or ctx is
// done. It waits on the goroutines Start launched, so call Start first.
func (p *Pool) Stop(ctx context.Context) error {
	p.shutdown()
	select {
	case <-p.exited:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *Pool) shutdown() {
	p.mu.Lock()
	p.stopped.Store(true)
	p.mu.Unlock()
	p.wake.Broadcast()
}

func (p *Pool) enqueue(q *Queue, fn func()) bool {
	if p.stopped.Load() {
		return false
	}
	if !q.scheduled {
		// Checked again under mu: a queue scheduled after the workers exit
		// would never drain.
		p.mu.Lock()
		if p.stopped.Load() {
			p.mu.Unlock()
			return false
		}
		p.runq = append(p.runq, q)
		p.mu.Unlock()
		p.wake.Signal()
		q.scheduled = true
	}
	q.tasks = append(q.tasks, fn)
	return true
}

// Now returns time.Now(), the clock the pool's timers run on.
func (p *Pool) Now() time.Time { return time.Now() }

func (p *Pool) afterFunc(d time.Duration, fn func()) clockTimer {
	return time.AfterFunc(d, fn)
}

// work drains runnable queues until the pool stops and the run queue is
// empty. The last worker to exit closes idle.
func (p *Pool) work(s *slot, idle chan<- struct{}) {
	stopped := false
	defer func() {
		if !stopped {
			// A task called runtime.Goexit and took this goroutine with it;
			// drain has already put its queue back. Replace the worker.
			go p.work(s, idle)
			return
		}
		if p.live.Add(-1) == 0 {
			close(idle)
		}
	}()
	var batch [drainSlice]func()
	for {
		p.mu.Lock()
		for len(p.runq) == 0 {
			if p.stopped.Load() {
				p.mu.Unlock()
				stopped = true
				return
			}
			p.wake.Wait()
		}
		q := p.runq[0]
		p.runq[0] = nil
		p.runq = p.runq[1:]
		p.mu.Unlock()
		p.drain(q, s, &batch)
	}
}

// drain runs up to drainSlice of q's tasks, then requeues q at the back of
// the run queue if more are pending. A queue with pending tasks is always
// either in the run queue or being drained, so accepted tasks run even while
// the pool is stopping.
func (p *Pool) drain(q *Queue, s *slot, batch *[drainSlice]func()) {
	q.mu.Lock()
	n := copy(batch[:], q.tasks)
	clear(q.tasks[:n])
	q.tasks = q.tasks[n:]
	q.mu.Unlock()

	i := 0
	// Deferred so it also runs when a task calls runtime.Goexit: the tasks
	// behind that one go back to the head of q, and q is requeued as usual.
	defer func() {
		q.mu.Lock()
		if i < n {
			q.tasks = append(slices.Clone(batch[i+1:n]), q.tasks...)
			clear(batch[i+1 : n])
		}
		more := len(q.tasks) > 0
		q.scheduled = more
		q.mu.Unlock()
		if more {
			p.mu.Lock()
			p.runq = append(p.runq, q)
			p.mu.Unlock()
			p.wake.Signal()
		}
	}()
	drainAs(q, func() {
		for ; i < n; i++ {
			fn := batch[i]
			batch[i] = nil
			p.run(s, q.id, fn)
		}
	})
}

// run runs fn for queue on the worker owning s. A panic is logged and
// contained so the worker survives; runtime.Goexit is logged and costs the
// worker goroutine, which work replaces. A slow task is logged with its
// final duration once it returns.
func (p *Pool) run(s *slot, queue string, fn func()) {
	start := time.Now()
	s.mu.Lock()
	s.queue, s.start = queue, start
	s.seq++
	s.mu.Unlock()
	returned := false
	defer func() {
		s.mu.Lock()
		s.start = time.Time{}
		s.mu.Unlock()
		if r := recover(); r != nil {
			p.log.Error().Str("queue", queue).Interface("panic", r).Msg("sim: recovered panic in queued task")
		} else if !returned {
			p.log.Error().Str("queue", queue).Msg("sim: queued task called runtime.Goexit; worker replaced")
		}
		if d := time.Since(start); d > slowTask {
			p.log.Warn().Str("queue", queue).Dur("elapsed", d).Msg("sim: slow task")
		}
	}()
	fn()
	returned = true
}

// watch logs, once per task, every task still running past slowTask, until
// idle closes. It is the only report of a task that never returns.
func (p *Pool) watch(idle <-chan struct{}) {
	tick := time.NewTicker(slowTask)
	defer tick.Stop()
	reported := make([]uint64, len(p.slots))
	for {
		select {
		case <-idle:
			return
		case now := <-tick.C:
			for i := range p.slots {
				s := &p.slots[i]
				s.mu.Lock()
				queue, start, seq := s.queue, s.start, s.seq
				s.mu.Unlock()
				if start.IsZero() || seq == reported[i] || now.Sub(start) <= slowTask {
					continue
				}
				reported[i] = seq
				p.log.Warn().Str("queue", queue).Dur("elapsed", now.Sub(start)).Msg("sim: task still running")
			}
		}
	}
}
