package gameservertest

import (
	"net"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// WithWriteStalls lets a suite stall the server's writes to one client
// (Server.StallWrites), standing in for a client that stopped reading while
// its receive window stays shut. The game listener then hands the server
// wrapped connections that pass every write through until stalled.
func WithWriteStalls() Option {
	return func(o *options) { o.writeStalls = &writeStalls{gates: make(map[string]*writeGate)} }
}

// writeStalls holds the write gate of every accepted game connection, by
// the client's address.
type writeStalls struct {
	mu    sync.Mutex
	gates map[string]*writeGate
}

// stallListener wraps each accepted connection in a write gate.
type stallListener struct {
	net.Listener
	stalls *writeStalls
}

func (l stallListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	g := &writeGate{Conn: c}
	l.stalls.mu.Lock()
	l.stalls.gates[c.RemoteAddr().String()] = g
	l.stalls.mu.Unlock()
	return g, nil
}

// writeGate is a connection whose writes block while shut is set, until it
// is closed; the write deadline does not cut that wait short.
type writeGate struct {
	net.Conn
	shut atomic.Pointer[chan struct{}]
}

func (g *writeGate) Write(p []byte) (int, error) {
	if shut := g.shut.Load(); shut != nil {
		<-*shut
	}
	return g.Conn.Write(p)
}

// StallWrites blocks every write the server makes to c from now on, frames
// still queueing behind it, until release runs; the test's end releases it
// too. The server must have booted WithWriteStalls.
func (s *Server) StallWrites(tb testing.TB, c *testsupport.ScriptedClient) (release func()) {
	tb.Helper()
	if s.writeStalls == nil {
		tb.Fatal("StallWrites needs WithWriteStalls")
	}
	s.writeStalls.mu.Lock()
	g := s.writeStalls.gates[c.LocalAddr().String()]
	s.writeStalls.mu.Unlock()
	if g == nil {
		tb.Fatalf("no game connection from %s", c.LocalAddr())
	}
	shut := make(chan struct{})
	g.shut.Store(&shut)
	var once sync.Once
	release = func() {
		once.Do(func() {
			g.shut.Store(nil)
			close(shut)
		})
	}
	cleanup(tb, release)
	return release
}
