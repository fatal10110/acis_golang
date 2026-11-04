package gameservertest

import (
	"sync/atomic"
	"testing"
	"time"
)

// expiredReadTimeout is the read deadline ExpireIdleReads hands the server: a
// wait under it runs out as soon as it begins.
const expiredReadTimeout = time.Nanosecond

// idleReads is the established-session read deadline the harness hands the
// game client link: zero keeps the server's 15 minutes. Any goroutine may
// read or set it.
type idleReads struct{ d atomic.Int64 }

func (r *idleReads) timeout() time.Duration { return time.Duration(r.d.Load()) }

// ExpireIdleReads makes every authenticated connection end idle right after
// the next frame it handles, as an idle client's does after 15 minutes, so no
// test waits the deadline out. It first waits until the server has handled
// every frame a client wrote: each read loop is then already waiting, under
// the deadline it took before the call, and the wait after the next frame is
// the one that runs out at once. A connection that had not yet begun its
// wait would otherwise expire it before reading the test's next frame.
//
// The driven clock does not count the expired wait as a read loop that has
// caught up: the loop still has to queue ServerClose and end the connection,
// so a client read waits for that frame instead of letting its virtual time
// run out first.
func (s *Server) ExpireIdleReads(tb testing.TB) {
	tb.Helper()
	s.AwaitHandled(tb)
	s.idleReads.d.Store(int64(expiredReadTimeout))
}
