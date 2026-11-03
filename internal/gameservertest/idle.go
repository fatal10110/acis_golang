package gameservertest

import (
	"sync/atomic"
	"time"
)

// idleReads is the established-session read deadline the harness hands the
// game client link: zero keeps the server's 15 minutes. Any goroutine may
// read or set it.
type idleReads struct{ d atomic.Int64 }

func (r *idleReads) timeout() time.Duration { return time.Duration(r.d.Load()) }

// ExpireIdleReads makes every authenticated connection's next wait for a
// frame run out at once, as an idle client's does after 15 minutes, so no
// test waits the deadline out. A read loop already waiting keeps its old
// deadline: its connection ends idle right after the next frame it handles.
func (s *Server) ExpireIdleReads() { s.idleReads.d.Store(int64(time.Nanosecond)) }
