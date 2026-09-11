package sim

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
)

// Pool drains queues on a fixed set of worker goroutines. A queue is never
// drained by two workers at once.
type Pool struct {
	log     zerolog.Logger
	workers int
	exited  chan struct{}

	// ponytail: one run-queue lock shared by every worker; per-worker deques
	// with stealing if it shows up in profiles.
	mu      sync.Mutex
	wake    sync.Cond
	runq    []*Queue
	stopped atomic.Bool // written under mu
}

// NewPool returns a pool of workers goroutines, GOMAXPROCS when workers is
// not positive. Posts are accepted before Start and run once it is called.
func NewPool(workers int, log zerolog.Logger) *Pool {
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	p := &Pool{log: log, workers: workers, exited: make(chan struct{})}
	p.wake.L = &p.mu
	return p
}

// NewQueue returns an open queue drained by p. id names it in logs.
func (p *Pool) NewQueue(id string) *Queue {
	return &Queue{id: id, exec: p}
}

// Start launches the workers. Cancelling ctx stops the pool as Stop does,
// without waiting.
func (p *Pool) Start(ctx context.Context) {
	var wg sync.WaitGroup
	for range p.workers {
		wg.Go(p.work)
	}
	go func() {
		wg.Wait()
		close(p.exited)
	}()
	context.AfterFunc(ctx, p.shutdown)
}

// Stop refuses further posts, lets the workers finish every task already
// accepted, and returns once they have exited or ctx is done. It waits on
// the workers Start launched, so call Start first.
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

func (p *Pool) afterFunc(d time.Duration, fn func()) clockTimer {
	return time.AfterFunc(d, fn)
}

func (p *Pool) work() {
	var batch [drainSlice]func()
	for {
		p.mu.Lock()
		for len(p.runq) == 0 {
			if p.stopped.Load() {
				p.mu.Unlock()
				return
			}
			p.wake.Wait()
		}
		q := p.runq[0]
		p.runq[0] = nil
		p.runq = p.runq[1:]
		p.mu.Unlock()
		p.drain(q, &batch)
	}
}

// drain runs up to drainSlice of q's tasks, then requeues q at the back of
// the run queue if more are pending. A queue with pending tasks is always
// either in the run queue or being drained, so accepted tasks run even while
// the pool is stopping.
func (p *Pool) drain(q *Queue, batch *[drainSlice]func()) {
	q.mu.Lock()
	n := copy(batch[:], q.tasks)
	clear(q.tasks[:n])
	q.tasks = q.tasks[n:]
	q.mu.Unlock()

	drainAs(q, func() {
		for i, fn := range batch[:n] {
			batch[i] = nil
			runTask(p.log, q.id, fn)
		}
	})

	q.mu.Lock()
	more := len(q.tasks) > 0
	q.scheduled = more
	q.mu.Unlock()
	if more {
		p.mu.Lock()
		p.runq = append(p.runq, q)
		p.mu.Unlock()
		p.wake.Signal()
	}
}
