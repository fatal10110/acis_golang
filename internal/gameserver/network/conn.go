package network

import (
	"net"
	"sync"

	"github.com/sirupsen/logrus"
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
	log      *logrus.Logger
	mu       sync.Mutex
	out      chan []byte
	closed   bool
	stopped  chan struct{}
	closeErr error
}

func newConn(c net.Conn, log *logrus.Logger) *Conn {
	if log == nil {
		log = logrus.StandardLogger()
	}
	conn := &Conn{
		Conn:    c,
		log:     log,
		out:     make(chan []byte, outboundBuffer),
		stopped: make(chan struct{}),
	}
	go conn.writeLoop()
	return conn
}

// writeLoop drains queued sends in order and only closes the
// underlying connection once the queue is empty and Close has been
// called, so a Send queued right before Close is never dropped. A
// panic while writing is recovered and logged so it disconnects only
// this client, never the process; the deferred cleanup still runs so
// Close never blocks forever waiting on stopped.
func (c *Conn) writeLoop() {
	defer func() {
		if r := recover(); r != nil {
			c.log.Errorf("game connection writer panic: %v", r)
		}
		c.closeErr = c.Conn.Close()
		close(c.stopped)
	}()
	for payload := range c.out {
		c.Conn.Write(payload)
	}
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
