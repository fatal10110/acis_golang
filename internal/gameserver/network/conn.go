package network

import (
	"net"
	"sync"
	"time"
	"unsafe"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/rs/zerolog"
)

// outboundHighWater is how much memory a Conn's unwritten backlog may pin
// before the peer is treated as stalled and the connection is aborted. Each
// frame is charged what it holds until written — its pooled buffer's
// capacity plus its queue slot (frameCost), not only its wire bytes — so a
// flood of tiny replies is bounded like a few large frames. Frames are never
// dropped individually: skipping one would desynchronize the client's cipher.
// 2 MiB is several thousand typical frames, well above a healthy burst such
// as an EnterWorld in a crowded town.
const outboundHighWater = 2 << 20

// queuedFrameBytes is the queue slot each backlog frame occupies.
const queuedFrameBytes = int(unsafe.Sizeof(wire.Frame{}))

// retainedQueueCap is the largest queue backing array a Conn keeps after a
// burst drains; larger ones are dropped so an idle connection doesn't hold
// its peak backlog's slots for life.
const retainedQueueCap = 1024

// frameCost is what frame counts against outboundHighWater.
func frameCost(frame wire.Frame) int {
	return frame.Footprint() + queuedFrameBytes
}

// Conn is one accepted game-client connection: a net.Conn plus an
// unbounded outbound FIFO drained by a single dedicated writer goroutine, so
// nothing but that goroutine ever calls Write on the underlying net.Conn and
// no sender ever blocks on a slow peer. The read side belongs to whatever
// handler Serve invokes.
type Conn struct {
	net.Conn
	log zerolog.Logger

	// mu guards queue, pending and closed. It is held only to append to or
	// swap out the queue, never across I/O.
	mu      sync.Mutex
	queue   []wire.Frame
	pending int // frameCost of every frame queued or being written
	closed  bool

	wake     chan struct{} // capacity 1: queue gained frames or closed was set
	stopping chan struct{}
	stopOnce sync.Once
	stopped  chan struct{}
	closeErr error
}

func newConn(c net.Conn, log zerolog.Logger) *Conn {
	conn := &Conn{
		Conn:     c,
		log:      log,
		wake:     make(chan struct{}, 1),
		stopping: make(chan struct{}),
		stopped:  make(chan struct{}),
	}
	go conn.writeLoop()
	return conn
}

// writeLoop drains queued sends in order for Close, so a frame queued right
// before Close is never dropped. abort instead stops this loop immediately
// and releases the backlog. A panic while writing is recovered and logged so it
// disconnects only this client, never the process; the deferred
// cleanup still runs so Close never blocks forever waiting on stopped.
//
// Once this loop exits early on a write error, later SendFrame calls fail
// without queueing because closed is set.
//
// Each iteration takes the whole backlog at once so a burst coalesces into
// one vectored net.Buffers write instead of one Write syscall per frame.
// batch and the queue swap backing arrays, so steady state allocates
// nothing; once a burst grew them past retainedQueueCap, they and bufs are
// dropped after it drains.
func (c *Conn) writeLoop() {
	defer func() {
		if r := recover(); r != nil {
			c.log.Error().Interface("panic", r).Msg("game connection writer panic")
		}
		c.stop()
		c.mu.Lock()
		c.closed = true
		releaseFrames(c.queue)
		c.queue = nil
		c.mu.Unlock()
		c.closeErr = c.Conn.Close()
		close(c.stopped)
	}()
	// batch and bufs are reused across iterations — this loop is their only
	// owner, so no other goroutine ever sees them.
	var batch []wire.Frame
	var bufs net.Buffers
	for {
		c.mu.Lock()
		batch, c.queue = c.queue, batch[:0]
		closed := c.closed
		c.mu.Unlock()
		if len(batch) == 0 {
			if closed {
				return
			}
			select {
			case <-c.stopping:
				return
			case <-c.wake:
			}
			continue
		}
		var cost int
		var err error
		bufs, cost, err = c.writeBatch(batch, bufs)
		c.mu.Lock()
		c.pending -= cost
		c.mu.Unlock()
		if err != nil {
			c.log.Warn().Err(err).Msg("game connection write failed, closing")
			return
		}
		if cap(batch) > retainedQueueCap {
			batch, bufs = nil, nil
		}
	}
}

// writeBatch writes every frame in batch as a single vectored
// net.Buffers write and releases all of them (win or lose) once the
// write attempt finishes. It reuses bufs' storage and returns it, possibly
// grown, for the next batch; WriteTo consumes a separate slice header, so
// the returned one keeps the whole backing array. cost is the batch's total
// frameCost, so the caller can retire it from pending either way.
func (c *Conn) writeBatch(batch []wire.Frame, bufs net.Buffers) (_ net.Buffers, cost int, _ error) {
	defer releaseFrames(batch)
	bufs = bufs[:0]
	for _, frame := range batch {
		bufs = append(bufs, frame.Bytes())
		cost += frameCost(frame)
	}
	if err := c.Conn.SetWriteDeadline(time.Now().Add(time.Minute)); err != nil {
		return bufs, cost, err
	}
	send := bufs
	_, err := send.WriteTo(c.Conn)
	return bufs, cost, err
}

// releaseFrames releases every frame and clears the slots so a reused
// backing array does not pin released buffers.
func releaseFrames(frames []wire.Frame) {
	for _, frame := range frames {
		frame.Release()
	}
	clear(frames)
}

// SendFrame queues a frame without blocking and calls release exactly once
// after the frame is written or dropped. It reports false when the
// connection is closed, or when this frame would push the unwritten backlog
// past outboundHighWater — the connection is aborted then, because dropping
// one ordered frame would desynchronize the client.
func (c *Conn) SendFrame(frame wire.Frame) bool {
	if frame.Err() != nil {
		frame.Release()
		return false
	}
	cost := frameCost(frame)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		frame.Release()
		return false
	}
	if c.pending+cost > outboundHighWater {
		c.mu.Unlock()
		frame.Release()
		c.abort()
		return false
	}
	c.queue = append(c.queue, frame)
	c.pending += cost
	c.mu.Unlock()
	c.signal()
	return true
}

func (c *Conn) signal() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// abort closes a connection without waiting for its writer to drain. It is
// used when a client stopped reading its ordered packet stream. A connection
// already closing is left to finish its own drain.
func (c *Conn) abort() {
	c.mu.Lock()
	wasClosed := c.closed
	c.closed = true
	c.mu.Unlock()
	if wasClosed {
		return
	}
	c.log.Warn().Int("high_water_bytes", outboundHighWater).Msg("outbound backlog over high-water mark, aborting connection")
	c.stop()
	_ = c.Conn.Close()
}

func (c *Conn) stop() {
	c.stopOnce.Do(func() { close(c.stopping) })
}

// Close stops accepting new sends, flushes any already queued, then
// closes the underlying connection. Safe to call more than once; every
// call blocks until the underlying connection is actually closed.
func (c *Conn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	c.signal()
	<-c.stopped
	return c.closeErr
}
