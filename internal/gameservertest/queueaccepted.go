package gameservertest

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// AwaitQueueAccepted waits until q has accepted more than n tasks: work a
// connection goroutine posts on its own, such as the detach that follows a
// logout, reached q while the test holds it. It fails tb if that takes longer
// than catchUpTimeout (a failure guard, not a pacing delay).
func (s *Server) AwaitQueueAccepted(tb testing.TB, q *sim.Queue, n uint64) {
	tb.Helper()
	for deadline := time.Now().Add(catchUpTimeout); q.Accepted() <= n; {
		if time.Now().After(deadline) {
			tb.Fatalf("queue accepted %d tasks, want more than %d", q.Accepted(), n)
		}
		time.Sleep(time.Millisecond)
	}
}
