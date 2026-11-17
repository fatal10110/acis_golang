package script

import "sync"

// offload runs the database work of the scheduled tasks on one goroutine it
// owns, in the order it was handed over, so that none of it holds the
// schedule's queue. mu guards jobs, pending and closed; cond wakes the
// goroutine when jobs or closed change, and a flush when pending drops.
type offload struct {
	mu      sync.Mutex
	cond    sync.Cond
	jobs    []func()
	pending int
	closed  bool
	done    chan struct{}
}

// newOffload starts an offload goroutine; close stops it.
func newOffload() *offload {
	o := &offload{done: make(chan struct{})}
	o.cond.L = &o.mu
	go o.run()
	return o
}

// push queues job behind the jobs already handed over. It reports false,
// without queueing job, once the offload is closed.
func (o *offload) push(job func()) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return false
	}
	o.jobs = append(o.jobs, job)
	o.pending++
	o.cond.Broadcast()
	return true
}

func (o *offload) run() {
	defer close(o.done)
	for {
		o.mu.Lock()
		for len(o.jobs) == 0 && !o.closed {
			o.cond.Wait()
		}
		if o.closed {
			o.mu.Unlock()
			return
		}
		job := o.jobs[0]
		o.jobs[0] = nil
		o.jobs = o.jobs[1:]
		o.mu.Unlock()

		job()

		o.mu.Lock()
		o.pending--
		o.cond.Broadcast()
		o.mu.Unlock()
	}
}

// flush returns once every job handed over before it has run or been
// dropped.
func (o *offload) flush() {
	o.mu.Lock()
	defer o.mu.Unlock()
	for o.pending > 0 {
		o.cond.Wait()
	}
}

// close refuses later jobs, drops those not yet started and waits for the
// running one to return.
func (o *offload) close() {
	o.mu.Lock()
	o.closed = true
	o.pending -= len(o.jobs)
	o.jobs = nil
	o.cond.Broadcast()
	o.mu.Unlock()
	<-o.done
}
