package sim

import (
	"container/heap"
	"sync"
	"time"
)

// Inline is a deterministic single-threaded executor for tests. Every queue
// it creates shares one FIFO, so tasks run in global post order, and timers
// follow a virtual clock that moves only on Advance. Post is safe from any
// goroutine but never runs a task; Run and Advance run them on the calling
// goroutine and must not be called from a task.
//
// Unlike Pool, Inline does not recover: a task's panic reaches the caller of
// Run or Advance, so a test fails instead of logging it. The queue is left
// idle and the tasks behind it run on the next call.
type Inline struct {
	mu     sync.Mutex
	at     time.Time // the virtual now
	tasks  []inlineTask
	timers vtimerHeap
	seq    uint64
}

type inlineTask struct {
	q  *Queue
	fn func()
}

// NewInline returns an idle loop whose clock reads start.
func NewInline(start time.Time) *Inline {
	return &Inline{at: start}
}

// NewQueue returns an open queue run by in. id names it in logs.
func (in *Inline) NewQueue(id string) *Queue {
	return &Queue{id: id, exec: in}
}

// Now returns the virtual time.
func (in *Inline) Now() time.Time {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.at
}

// Run runs posted tasks in post order, including the ones they post, until
// none remain.
func (in *Inline) Run() {
	for {
		in.mu.Lock()
		if len(in.tasks) == 0 {
			in.mu.Unlock()
			return
		}
		t := in.tasks[0]
		in.tasks[0] = inlineTask{}
		in.tasks = in.tasks[1:]
		in.mu.Unlock()
		drainAs(t.q, t.fn)
	}
}

// Advance runs pending tasks, then moves the clock forward by d. Each timer
// due by the new time fires at its own deadline, in deadline order (ties in
// arming order), and the tasks it posts run before the next timer fires.
func (in *Inline) Advance(d time.Duration) {
	in.Run()
	in.mu.Lock()
	end := in.at.Add(d)
	for len(in.timers) > 0 && !in.timers[0].at.After(end) {
		v := heap.Pop(&in.timers).(*vtimer)
		if v.at.After(in.at) { // a negative delay fires now, not in the past
			in.at = v.at
		}
		in.mu.Unlock()
		v.fn()
		in.Run()
		in.mu.Lock()
	}
	in.at = end
	in.mu.Unlock()
}

func (in *Inline) enqueue(q *Queue, fn func()) bool {
	in.mu.Lock()
	in.tasks = append(in.tasks, inlineTask{q: q, fn: fn})
	in.mu.Unlock()
	return true
}

func (in *Inline) afterFunc(d time.Duration, fn func()) clockTimer {
	v := &vtimer{in: in, fn: fn, index: -1}
	v.Reset(d)
	return v
}

// vtimer is a timer on the Inline clock. Its fields are guarded by in.mu.
type vtimer struct {
	in    *Inline
	fn    func()
	at    time.Time
	seq   uint64
	index int // position in in.timers, -1 when not armed
}

func (v *vtimer) Stop() bool {
	v.in.mu.Lock()
	defer v.in.mu.Unlock()
	if v.index < 0 {
		return false
	}
	heap.Remove(&v.in.timers, v.index)
	return true
}

func (v *vtimer) Reset(d time.Duration) bool {
	in := v.in
	in.mu.Lock()
	defer in.mu.Unlock()
	armed := v.index >= 0
	if armed {
		heap.Remove(&in.timers, v.index)
	}
	in.seq++
	v.at, v.seq = in.at.Add(d), in.seq
	heap.Push(&in.timers, v)
	return armed
}

type vtimerHeap []*vtimer

func (h vtimerHeap) Len() int { return len(h) }

func (h vtimerHeap) Less(i, j int) bool {
	if !h[i].at.Equal(h[j].at) {
		return h[i].at.Before(h[j].at)
	}
	return h[i].seq < h[j].seq
}

func (h vtimerHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index, h[j].index = i, j
}

func (h *vtimerHeap) Push(x any) {
	v := x.(*vtimer)
	v.index = len(*h)
	*h = append(*h, v)
}

func (h *vtimerHeap) Pop() any {
	old := *h
	v := old[len(old)-1]
	old[len(old)-1] = nil
	*h = old[:len(old)-1]
	v.index = -1
	return v
}
