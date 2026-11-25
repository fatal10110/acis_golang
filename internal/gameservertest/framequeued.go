package gameservertest

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// AwaitFrameQueued waits until the server has queued c a frame c has not
// read yet, without waiting for the other connections to catch up: a suite
// that holds one connection's work learns that another's answer went out,
// then lets the held work go before it reads. It fails tb if that takes
// longer than catchUpTimeout (a failure guard, not a pacing delay).
func (s *Server) AwaitFrameQueued(tb testing.TB, c *testsupport.ScriptedClient) {
	tb.Helper()
	for deadline := time.Now().Add(catchUpTimeout); ; {
		if ct := s.traffic.conn(c.LocalAddr()); ct != nil && ct.sent.Load() > c.Received() {
			return
		}
		if time.Now().After(deadline) {
			tb.Fatalf("no frame queued to %s", c.LocalAddr())
		}
		time.Sleep(time.Millisecond)
	}
}
