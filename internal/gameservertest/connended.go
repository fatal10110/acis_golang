package gameservertest

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// AwaitConnectionEnded waits until the server is done with c's connection:
// its handler returned, so the player it held has detached and the end of
// the connection was reported to the login server, or deliberately not.
// It fails tb if that takes longer than loginLinkSyncWait (a failure guard,
// not a pacing delay).
func (s *Server) AwaitConnectionEnded(tb testing.TB, c *testsupport.ScriptedClient) {
	tb.Helper()
	for deadline := time.Now().Add(loginLinkSyncWait); ; {
		if ct := s.traffic.conn(c.LocalAddr()); ct != nil && ct.done.Load() {
			return
		}
		if time.Now().After(deadline) {
			tb.Fatalf("connection from %s still handled", c.LocalAddr())
		}
		time.Sleep(time.Millisecond)
	}
}
