package script

import (
	"runtime"
	"slices"
	"testing"
)

// TestOffloadRunsInOrderAndCloseDropsTheRest: jobs run one at a time in the
// order handed over and flush waits for them; close waits for the running
// job, drops those not yet started and refuses later ones.
func TestOffloadRunsInOrderAndCloseDropsTheRest(t *testing.T) {
	o := newOffload()
	var ran []int
	for i := range 3 {
		if !o.push(func() { ran = append(ran, i) }) {
			t.Fatalf("job %d refused", i)
		}
	}
	o.flush()
	if !slices.Equal(ran, []int{0, 1, 2}) {
		t.Fatalf("ran %v, want 0 1 2", ran)
	}

	started, release := make(chan struct{}), make(chan struct{})
	o.push(func() {
		close(started)
		<-release
		ran = append(ran, 3)
	})
	o.push(func() { ran = append(ran, 4) })
	<-started
	closed := make(chan struct{})
	go func() {
		o.close()
		close(closed)
	}()
	for !o.isClosed() {
		runtime.Gosched()
	}
	close(release)
	<-closed
	o.flush()
	if o.push(func() { ran = append(ran, 5) }) {
		t.Fatal("closed offload took a job")
	}
	if !slices.Equal(ran, []int{0, 1, 2, 3}) {
		t.Fatalf("ran %v, want 0 1 2 3", ran)
	}
}

func (o *offload) isClosed() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.closed
}
