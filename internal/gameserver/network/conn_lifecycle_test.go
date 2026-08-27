package network

import (
	"testing"
	"time"
)

// TestConnStopUnblocksFullQueueSender catches a writer-exit deadlock: stop
// must signal a sender blocked on a full outbound queue before taking mu.
func TestConnStopUnblocksFullQueueSender(t *testing.T) {
	c := &Conn{
		out:      make(chan queuedWrite, 1),
		stopping: make(chan struct{}),
	}
	c.out <- queuedWrite{}

	sent := make(chan bool, 1)
	go func() { sent <- c.send(queuedWrite{}) }()
	select {
	case <-sent:
		t.Fatal("send returned before the full queue was stopped")
	case <-time.After(20 * time.Millisecond):
	}

	stopped := make(chan struct{})
	go func() {
		c.stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(time.Second):
		// Unblock the old deadlocking implementation before failing the test.
		<-c.out
		<-sent
		<-stopped
		t.Fatal("stop did not signal the full-queue sender")
	}

	select {
	case ok := <-sent:
		if ok {
			t.Fatal("send succeeded after stop")
		}
	case <-time.After(20 * time.Millisecond):
		t.Fatal("full-queue sender did not observe stop")
	}
}
