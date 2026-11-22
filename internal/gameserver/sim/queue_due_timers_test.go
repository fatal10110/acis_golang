package sim

import (
	"testing"
	"time"
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
