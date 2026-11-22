package sim

import (
	"testing"

	"github.com/rs/zerolog"
)

// TestPoolPostDrainCycleAllocatesNothing: once a queue and the run queue
// have been through one post and drain, the next post-then-drain cycle
// reuses their buffers.
func TestPoolPostDrainCycleAllocatesNothing(t *testing.T) {
	if simdebugBuild {
		t.Skip("simdebug owner bookkeeping allocates per drain; the budget describes the production build")
	}
	p := startPool(t, 1, zerolog.Nop())
	q := p.NewQueue("q")
	done := make(chan struct{})
	task := func() { done <- struct{}{} }
	cycle := func() {
		if !q.Post(task) {
			t.Fatal("Post refused")
		}
		<-done
	}
	cycle()
	if n := testing.AllocsPerRun(200, cycle); n != 0 {
		t.Fatalf("a warm post-then-drain cycle allocates %v times, want 0", n)
	}
}

// TestPoolBurstDrainCycleAllocatesNothing: a warm queue taking a burst that
// fills one drain slice, posted while its worker is busy, reuses its buffer.
func TestPoolBurstDrainCycleAllocatesNothing(t *testing.T) {
	if simdebugBuild {
		t.Skip("simdebug owner bookkeeping allocates per drain; the budget describes the production build")
	}
	p := startPool(t, 1, zerolog.Nop())
	q := p.NewQueue("q")
	// The holding task may still be pending when the burst lands, so the
	// burst leaves it room within idleTaskCap.
	const burst = idleTaskCap - 1
	gate := make(chan struct{})
	done := make(chan struct{}, burst)
	hold := func() { <-gate }
	task := func() { done <- struct{}{} }
	cycle := func() {
		q.Post(hold)
		for range burst {
			if !q.Post(task) {
				t.Fatal("Post refused")
			}
		}
		gate <- struct{}{}
		for range burst {
			<-done
		}
	}
	cycle()
	if n := testing.AllocsPerRun(50, cycle); n != 0 {
		t.Fatalf("a warm burst cycle allocates %v times, want 0", n)
	}
}

// TestPoolReleasesAnIdleQueuesBurstBuffer: a burst larger than idleTaskCap
// does not stay pinned to the queue once it is drained.
func TestPoolReleasesAnIdleQueuesBurstBuffer(t *testing.T) {
	p := startPool(t, 1, zerolog.Nop())
	q := p.NewQueue("q")
	gate := make(chan struct{})
	const burst = 4 * idleTaskCap
	q.Post(func() { <-gate })
	for range burst {
		q.Post(func() {})
	}
	drained := make(chan struct{})
	q.mu.Lock()
	grown := cap(q.tasks.buf)
	q.mu.Unlock()
	if grown <= idleTaskCap {
		t.Fatalf("burst grew the buffer to %d, want above %d", grown, idleTaskCap)
	}
	q.Post(func() { close(drained) })
	close(gate)
	<-drained
	// The last task has run; its drain's bookkeeping finishes under q.mu
	// before the next drain of q or the pool's Stop returns.
	stopPool(t, p)
	q.mu.Lock()
	defer q.mu.Unlock()
	if c := cap(q.tasks.buf); c > idleTaskCap {
		t.Fatalf("idle queue keeps a %d-task buffer, want at most %d", c, idleTaskCap)
	}
}
