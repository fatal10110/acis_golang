package network

import (
	"net"
	"sync"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/rs/zerolog"
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
	log      zerolog.Logger
	mu       sync.RWMutex
	out      chan queuedWrite
	closed   bool
	stopping chan struct{}
	stopped  chan struct{}
	closeErr error
}

type queuedWrite struct {
	frame wire.Frame
}

func newConn(c net.Conn, log zerolog.Logger) *Conn {
	conn := &Conn{
		Conn:     c,
		log:      log,
		out:      make(chan queuedWrite, outboundBuffer),
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
//
// Each iteration greedily drains c.out (bounded by outboundBuffer) after
// its first queued frame so a burst coalesces into one vectored
// net.Buffers write instead of one Write syscall per frame. Idle
// behavior is unchanged: with nothing queued, the loop blocks on the
// range receive.
func (c *Conn) writeLoop() {
	defer func() {
		if r := recover(); r != nil {
			c.log.Error().Interface("panic", r).Msg("game connection writer panic")
		}
		close(c.stopping)
		c.releaseQueued()
		c.closeErr = c.Conn.Close()
		close(c.stopped)
	}()
	// batch and bufs are reused across iterations — this loop is their only
	// owner, so no other goroutine ever sees them.
	var batch []queuedWrite
	var bufs net.Buffers
	for queued := range c.out {
		batch = c.drainBatch(batch[:0], queued)
		var err error
		if bufs, err = c.writeBatch(batch, bufs); err != nil {
			c.log.Warn().Err(err).Msg("game connection write failed, closing")
			return
		}
	}
}

// drainBatch appends first plus any further queued frames already
// sitting in c.out, without blocking, up to outboundBuffer total so one
// slow reader can't build an unbounded batch.
func (c *Conn) drainBatch(batch []queuedWrite, first queuedWrite) []queuedWrite {
	batch = append(batch, first)
	for len(batch) < outboundBuffer {
		select {
		case queued, ok := <-c.out:
			if !ok {
				return batch
			}
			batch = append(batch, queued)
		default:
			return batch
		}
	}
	return batch
}

// writeBatch writes every frame in batch as a single vectored
// net.Buffers write and releases all of them (win or lose) once the
// write attempt finishes. It reuses bufs' storage and returns it, possibly
// grown, for the next batch; WriteTo consumes a separate slice header, so
// the returned one keeps the whole backing array.
func (c *Conn) writeBatch(batch []queuedWrite, bufs net.Buffers) (net.Buffers, error) {
	defer func() {
		for _, queued := range batch {
			queued.frame.Release()
		}
	}()
	bufs = bufs[:0]
	for _, queued := range batch {
		bufs = append(bufs, queued.frame.Bytes())
	}
	send := bufs
	_, err := send.WriteTo(c.Conn)
	return bufs, err
}

func (c *Conn) releaseQueued() {
	for {
		select {
		case queued, ok := <-c.out:
			if !ok {
				return
			}
			queued.frame.Release()
		default:
			return
		}
	}
}

// Send queues payload to be written by this connection's writer goroutine.
// If Send returns true, the caller has handed off ownership and must not
// mutate payload. It returns false without blocking if the connection is
// already closed or its writer has stopped.
func (c *Conn) Send(payload []byte) bool {
	return c.send(queuedWrite{frame: wire.BorrowedFrame(payload)})
}

// SendFrame queues an owned frame and calls release exactly once after the
// frame is written or dropped because the connection is closed.
func (c *Conn) SendFrame(frame wire.Frame) bool {
	if c.send(queuedWrite{frame: frame}) {
		return true
	}
	frame.Release()
	return false
}

func (c *Conn) send(queued queuedWrite) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return false
	}
	select {
	case <-c.stopping:
		return false
	default:
	}
	select {
	case c.out <- queued:
		return true
	case <-c.stopping:
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
