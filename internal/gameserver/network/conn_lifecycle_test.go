package network

import (
	"testing"
	"time"
)

// TestConnStopWaitsForInFlightSend catches a shutdown race that could leave a
// successfully queued owned frame behind after writeLoop drained c.out.
func TestConnStopWaitsForInFlightSend(t *testing.T) {
	c := &Conn{stopping: make(chan struct{})}
	c.mu.RLock() // Model an enqueue that passed its closed/stopping checks.

	stopped := make(chan struct{})
	go func() {
		c.stop()
		close(stopped)
	}()

	select {
	case <-stopped:
		t.Fatal("stop completed while an enqueue held the read lock")
	case <-time.After(20 * time.Millisecond):
	}

	c.mu.RUnlock()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("stop did not complete after the enqueue released the read lock")
	}
}
