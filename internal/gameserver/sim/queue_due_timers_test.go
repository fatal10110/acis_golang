package sim

import (
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// TestDueTimersCountsOnlyExpiriesTheClockHasNotDelivered: an armed timer is
// due once its deadline has come and stays so until the clock posts its
// expiry; a timer set further out, a stopped one and one already delivered
// are not.
func TestDueTimersCountsOnlyExpiriesTheClockHasNotDelivered(t *testing.T) {
	in := NewInline(epoch)
	q := in.NewQueue("q")
	if q.DueTimers(in.Now()) {
		t.Fatal("a queue without timers reports one due")
	}
	later := q.After(time.Second, func() {})
	if q.DueTimers(in.Now()) {
		t.Fatal("a timer a second away reports due now")
	}
	if !q.DueTimers(in.Now().Add(time.Second)) {
		t.Fatal("the timer is not due at its deadline")
	}
	later.Stop()
	if q.DueTimers(in.Now().Add(time.Second)) {
		t.Fatal("a stopped timer reports due")
	}

	ran := false
	q.After(0, func() { ran = true })
	if !q.DueTimers(in.Now()) {
		t.Fatal("a timer armed already due is not reported before the clock delivers it")
	}
	in.Advance(0)
	if !ran || q.DueTimers(in.Now()) {
		t.Fatalf("after delivery ran = %v, due = %v; want true, false", ran, q.DueTimers(in.Now()))
	}
}

// TestDueTimersIgnoresATickAlreadyPostedToABlockedQueue: a ticker whose expiry
// sits posted behind a running task is accepted work, so it is not reported
// due however far the probe's clock is past its next deadline.
func TestDueTimersIgnoresATickAlreadyPostedToABlockedQueue(t *testing.T) {
	p := startPool(t, 1, zerolog.Nop())
	q := p.NewQueue("q")
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock) // runs before startPool's stop, so a failure does not stall it
	q.Post(func() { <-release })

	ticker := q.Every(2*time.Millisecond, func() {})
	tm := (*Timer)(ticker)
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(time.Millisecond) {
		q.mu.Lock()
		queued := tm.queued
		q.mu.Unlock()
		if queued {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("ticker never queued a tick while the queue was blocked")
		}
	}
	if q.DueTimers(time.Now().Add(time.Hour)) {
		t.Fatal("a tick already posted to the blocked queue reports due")
	}
	ticker.Stop()
	unblock()
}
