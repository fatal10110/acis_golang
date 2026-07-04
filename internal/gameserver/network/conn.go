package network

import (
	"net"
	"sync"
)

// outboundBuffer is how many pending writes a Conn queues before Send
// starts blocking the caller.
const outboundBuffer = 64

// Conn is one accepted game-client connection: a net.Conn plus an
// outbound queue drained by a single dedicated writer goroutine, so
// nothing but that goroutine ever calls Write on the underlying
// net.Conn. The read side belongs to whatever handler Serve invokes.
type Conn struct {
	net.Conn
	mu       sync.Mutex
	out      chan []byte
	closed   bool
	stopped  chan struct{}
	closeErr error
}

func newConn(c net.Conn) *Conn {
	conn := &Conn{
		Conn:    c,
		out:     make(chan []byte, outboundBuffer),
		stopped: make(chan struct{}),
	}
	go conn.writeLoop()
	return conn
}

// writeLoop drains queued sends in order and only closes the
// underlying connection once the queue is empty and Close has been
// called, so a Send queued right before Close is never dropped.
func (c *Conn) writeLoop() {
	for payload := range c.out {
		c.Conn.Write(payload)
	}
	c.closeErr = c.Conn.Close()
	close(c.stopped)
}

// Send queues payload to be written by this connection's writer
// goroutine. It returns false without blocking if the connection is
// already closed.
func (c *Conn) Send(payload []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	c.out <- payload
	return true
}

// Close stops accepting new sends, flushes any already queued, then
// closes the underlying connection. Safe to call more than once; every
// call blocks until the underlying connection is actually closed.
func (c *Conn) Close() error {
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		close(c.out)
	}
	c.mu.Unlock()
	<-c.stopped
	return c.closeErr
}
