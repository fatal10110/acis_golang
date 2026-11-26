package sim

import (
	"testing"

	"github.com/rs/zerolog"
)

// AssertNotOnQueue panics on a queue only in the simdebug build; the
// production build checks nothing.
func TestAssertNotOnQueuePanicsOnlyOnAQueue(t *testing.T) {
	p := startPool(t, 2, zerolog.Nop())
	q, other := p.NewQueue("q"), p.NewQueue("other")

	if panics(AssertNotOnQueue) {
		t.Fatal("AssertNotOnQueue panicked off every queue")
	}
	onTask := make(chan bool)
	q.Post(func() { onTask <- panics(AssertNotOnQueue) })
	if got := <-onTask; got != simdebugBuild {
		t.Fatalf("AssertNotOnQueue in a queue's task panicked = %v, want %v", got, simdebugBuild)
	}
	owned := false
	RunOwned(other, func() { owned = panics(AssertNotOnQueue) })
	if owned != simdebugBuild {
		t.Fatalf("AssertNotOnQueue under RunOwned panicked = %v, want %v", owned, simdebugBuild)
	}
	if panics(AssertNotOnQueue) {
		t.Fatal("AssertNotOnQueue panicked once RunOwned returned")
	}
}
