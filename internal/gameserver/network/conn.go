package network

import (
	"net"
	"sync"

	"github.com/sirupsen/logrus"
)

// outboundBuffer is how many pending writes a Conn queues before Send
// starts blocking the caller.
const outboundBuffer = 64

type queuedSend struct {
	bytes   []byte
	release func()
}

func (s queuedSend) done() {
	if s.release != nil {
		s.release()
	}
}

// Conn is one accepted game-client connection: a net.Conn plus an
// outbound queue drained by a single dedicated writer goroutine, so
// nothing but that goroutine ever calls Write on the underlying
// net.Conn. The read side belongs to whatever handler Serve invokes.
type Conn struct {
	net.Conn
	log      *logrus.Logger
	mu       sync.RWMutex
	out      chan queuedSend
	closed   bool
	stopping chan struct{}
	stopped  chan struct{}
	closeErr error
}

func newConn(c net.Conn, log *logrus.Logger) *Conn {
	if log == nil {
		log = logrus.StandardLogger()
	}
	conn := &Conn{
		Conn:     c,
		log:      log,
		out:      make(chan queuedSend, outboundBuffer),
		stopping: make(chan struct{}),
		stopped:  make(chan struct{}),
	}
	go conn.writeLoop()
	return conn
}

// writeLoop drains queued sends in order and only closes the
// underlying connection once the queue is empty and Close has been
// called (or a write fails), so a Send queued right before Close is
// never dropped. A panic while writing is recovered and logged so it
// disconnects only this client, never the process; the deferred
// cleanup still runs so Close never blocks forever waiting on stopped.
//
// Once this loop exits early on a write error, later Send calls fail
// without queueing because nothing drains c.out any more.
func (c *Conn) writeLoop() {
	defer func() {
		if r := recover(); r != nil {
			c.log.Errorf("game connection writer panic: %v", r)
		}
		close(c.stopping)
		c.releaseQueued()
		c.closeErr = c.Conn.Close()
		close(c.stopped)
	}()
	for payload := range c.out {
		if err := c.write(payload); err != nil {
			c.log.Warnf("game connection write failed, closing: %v", err)
			return
		}
	}
}

func (c *Conn) releaseQueued() {
	for {
		select {
		case payload, ok := <-c.out:
			if !ok {
				return
			}
			payload.done()
		default:
			return
		}
	}
}

func (c *Conn) write(payload queuedSend) (err error) {
	defer payload.done()
	_, err = c.Conn.Write(payload.bytes)
	return err
}

// Send queues payload to be written by this connection's writer goroutine.
// If Send returns true, the write loop owns payload until the write attempt
// finishes. It returns false without blocking if the connection is already
// closed or its writer has stopped.
func (c *Conn) Send(payload []byte) bool {
	return c.send(payload, nil)
}

func (c *Conn) send(payload []byte, release func()) bool {
	select {
	case <-c.stopping:
		if release != nil {
			release()
		}
		return false
	default:
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		if release != nil {
			release()
		}
		return false
	}
	select {
	case c.out <- queuedSend{bytes: payload, release: release}:
		return true
	case <-c.stopping:
		if release != nil {
			release()
		}
		return false
	}
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
