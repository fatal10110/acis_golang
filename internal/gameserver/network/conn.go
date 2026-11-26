package network

import (
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/rs/zerolog"
)

// outboundHighWater is how large a Conn's unwritten backlog may grow — its
// wire bytes plus a queue slot per frame (backlog.charge) — before the peer is
// treated as stalled and the connection is aborted. Frames are never dropped
// individually: skipping one would desynchronize the client's cipher. 2 MiB is
// several thousand typical frames (a character or NPC appearance is a few
// hundred bytes), well above a healthy burst such as an EnterWorld in a
// crowded town.
const outboundHighWater = 2 << 20

// outboundPinnedLimit bounds the memory the same backlog pins (backlog.pinned).
// A frame built in a reused pooled writer holds that writer's whole capacity,
// up to 8 KiB, until it is written, however few bytes it carries. Charging
// that capacity against outboundHighWater lets pool reuse cut the backlog a
// healthy burst may reach to a few hundred frames; not bounding it at all
// lets a peer that stops reading pin hundreds of MiB through a flood of tiny
// replies. 16 MiB still admits about two thousand frames that each pin a
// full-size pooled writer.
const outboundPinnedLimit = 16 << 20

// queuedFrameBytes is the queue slot each backlog frame occupies.
const queuedFrameBytes = int(unsafe.Sizeof(wire.Frame{}))

// retainedQueueCap is the largest queue backing array a Conn keeps after a
// burst drains; larger ones are dropped so an idle connection doesn't hold
// its peak backlog's slots for life.
const retainedQueueCap = 1024

// connWriteTimeout bounds one vectored write to the client: the writer
// goroutine is the only one blocked on a peer that stops reading, and this
// deadline is what frees it.
const connWriteTimeout = time.Minute

// abortTopOpcodes is how many of the most frequent queued opcodes the
// backlog abort log names.
const abortTopOpcodes = 5

// backlog totals the frames a Conn has queued or is writing.
type backlog struct {
	frames int
	wire   int // bytes the frames put on the wire
	pinned int // memory the frames hold: each one's Footprint plus queue slot
}

func (b *backlog) add(frame wire.Frame) {
	b.frames++
	b.wire += len(frame.Bytes())
	b.pinned += frame.Footprint() + queuedFrameBytes
}

func (b *backlog) retire(done backlog) {
	b.frames -= done.frames
	b.wire -= done.wire
	b.pinned -= done.pinned
}

// charge is what the backlog counts against outboundHighWater.
func (b backlog) charge() int {
	return b.wire + b.frames*queuedFrameBytes
}

// admits reports whether b stays within outboundHighWater and
// outboundPinnedLimit with frame added.
func (b backlog) admits(frame wire.Frame) bool {
	b.add(frame)
	return b.charge() <= outboundHighWater && b.pinned <= outboundPinnedLimit
}

// Conn is one accepted game-client connection: a net.Conn plus an
// unbounded outbound FIFO drained by a single dedicated writer goroutine, so
// nothing but that goroutine ever calls Write on the underlying net.Conn and
// no sender ever blocks on a slow peer. The read side belongs to whatever
// handler Serve invokes.
type Conn struct {
	net.Conn
	log zerolog.Logger
	// rejects reports frames SendFrame and sendLast refuse as invalid.
	rejects *frameRejectReporter

	// mu guards queue through aborted. It is held only to append to or swap
	// out the queue, never across I/O.
	mu    sync.Mutex
	queue []wire.Frame
	// queueOps is the cleartext opcode of each frame in queue, which the
	// backlog abort log reports; queue itself may already be encrypted.
	queueOps []byte
	pending  backlog // every frame queued or being written
	// account is who the client authenticated as, for the abort log.
	account string
	sealed  bool // sendLast queued the final frame; later sends are dropped
	closed  bool
	aborted bool // the backlog limit closed the connection

	wake     chan struct{} // capacity 1: queue gained frames or closed was set
	stopping chan struct{}
	stopOnce sync.Once
	stopped  chan struct{}
	closeErr error
	// wbufs is the slice header the writer hands to net.Buffers.WriteTo;
	// only writeLoop touches it.
	wbufs net.Buffers

	// observeSend, when set, sees each outbound payload on the goroutine
	// that queues it; see ObserveSends.
	observeSend func(payload []byte)
	// observeRead, when set, runs as each wait for an inbound frame begins,
	// after its read deadline is set; see ObserveReads.
	observeRead func(timeout time.Duration)
	// observePersistWait, when set, brackets each wait for queued saves;
	// see ObservePersistWaits.
	observePersistWait func(owners []int32) (done func())
}

func newConn(c net.Conn, log zerolog.Logger, rejects *frameRejectReporter) *Conn {
	conn := &Conn{
		Conn:     c,
		log:      log,
		rejects:  rejects,
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
		c.queue, c.queueOps = nil, nil
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
		if cap(c.queueOps) > retainedQueueCap {
			c.queueOps = nil
		} else {
			c.queueOps = c.queueOps[:0]
		}
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
		var written backlog
		var err error
		bufs, written, err = c.writeBatch(batch, bufs)
		c.mu.Lock()
		c.pending.retire(written)
		aborted := c.aborted
		c.mu.Unlock()
		if err != nil {
			// A write cut off by the backlog abort closing the socket was
			// already reported by that abort. A peer that already left
			// (broken pipe, reset) is an ordinary disconnect; any other
			// write failure (a timeout, an unknown error) stays loud.
			if !aborted {
				ev := c.log.Warn()
				if peerGone(err) {
					ev = c.log.Debug()
				}
				ev.Err(err).Msg("game connection write failed, closing")
			}
			return
		}
		if cap(batch) > retainedQueueCap {
			batch, bufs = nil, nil
		}
	}
}

// peerGone reports a write that failed because the peer had already closed
// or reset its end of the connection.
func peerGone(err error) bool {
	return errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET)
}

// writeBatch writes every frame in batch as a single vectored
// net.Buffers write and releases all of them (win or lose) once the
// write attempt finishes. It reuses bufs' storage and returns it, possibly
// grown, for the next batch; WriteTo consumes a separate slice header, so
// the returned one keeps the whole backing array. written totals the batch,
// so the caller can retire it from pending either way.
func (c *Conn) writeBatch(batch []wire.Frame, bufs net.Buffers) (_ net.Buffers, written backlog, _ error) {
	defer releaseFrames(batch)
	bufs = bufs[:0]
	for _, frame := range batch {
		bufs = append(bufs, frame.Bytes())
		written.add(frame)
	}
	if err := c.Conn.SetWriteDeadline(time.Now().Add(connWriteTimeout)); err != nil {
		return bufs, written, err
	}
	// WriteTo consumes the header it is called on; calling it on a field
	// rather than a local keeps that header off the heap.
	c.wbufs = bufs
	_, err := c.wbufs.WriteTo(c.Conn)
	c.wbufs = nil
	return bufs, written, err
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
// past outboundHighWater or outboundPinnedLimit — the connection is aborted
// then, because dropping one ordered frame would desynchronize the client.
func (c *Conn) SendFrame(frame wire.Frame) bool {
	return c.enqueue(frame, frameOpcode(frame.Bytes()), false)
}

// sendLast queues frame as the last one the peer receives: every later
// SendFrame and sendLast is dropped. Frames queued ahead of it are still
// written, and Close still flushes it before closing the socket. It reports
// false, releasing frame, when the connection is already sealed or closed.
// The final frame is never refused for the backlog limits.
func (c *Conn) sendLast(frame wire.Frame) bool {
	return c.enqueue(frame, frameOpcode(frame.Bytes()), true)
}

// frameOpcode is the opcode of a cleartext frame, or 0 when it has none.
func frameOpcode(frame []byte) byte {
	if len(frame) <= wire.FrameHeaderSize {
		return 0
	}
	return frame[wire.FrameHeaderSize]
}

// enqueue is SendFrame, or sendLast when last is set, for a frame whose
// cleartext opcode is op.
func (c *Conn) enqueue(frame wire.Frame, op byte, last bool) bool {
	if err := frame.Err(); err != nil {
		frame.Release()
		c.rejects.report("connection send", err)
		return false
	}
	c.mu.Lock()
	if c.closed || c.sealed {
		c.mu.Unlock()
		frame.Release()
		return false
	}
	if !last && !c.pending.admits(frame) {
		c.abort(frame, op) // unlocks mu
		return false
	}
	c.queue = append(c.queue, frame)
	c.queueOps = append(c.queueOps, op)
	c.pending.add(frame)
	c.sealed = last
	c.mu.Unlock()
	c.signal()
	return true
}

// abort closes a connection whose peer stopped reading its ordered packet
// stream, without waiting for its writer to drain, and releases rejected, the
// frame that did not fit. It is called with mu held by the sender that found
// the backlog over its limits, and unlocks it.
func (c *Conn) abort(rejected wire.Frame, op byte) {
	c.closed, c.aborted = true, true
	account, held, queued := c.account, c.pending, len(c.queue)
	top := topOpcodes(c.queueOps, abortTopOpcodes)
	c.mu.Unlock()

	c.log.Warn().
		Str("account", account).
		Stringer("addr", c.RemoteAddr()).
		Int("frames", held.frames).
		Int("frames_queued", queued).
		Int("wire_bytes", held.wire).
		Int("charged_bytes", held.charge()).
		Int("pinned_bytes", held.pinned).
		Str("top_opcodes", top).
		Str("rejected_opcode", fmt.Sprintf("0x%02x", op)).
		Int("rejected_wire_bytes", len(rejected.Bytes())).
		Int("rejected_pinned_bytes", rejected.Footprint()+queuedFrameBytes).
		Int("high_water_bytes", outboundHighWater).
		Int("pinned_limit_bytes", outboundPinnedLimit).
		Msg("outbound backlog over high-water mark, aborting connection")
	rejected.Release()
	c.stop()
	_ = c.Conn.Close()
}

// topOpcodes lists the n most frequent opcodes in ops, most frequent first,
// as "0x03=812,0x16=40".
func topOpcodes(ops []byte, n int) string {
	var counts [256]int
	for _, op := range ops {
		counts[op]++
	}
	var seen []int
	for op, count := range counts {
		if count > 0 {
			seen = append(seen, op)
		}
	}
	slices.SortStableFunc(seen, func(a, b int) int { return counts[b] - counts[a] })
	var out strings.Builder
	for i, op := range seen[:min(n, len(seen))] {
		if i > 0 {
			out.WriteByte(',')
		}
		fmt.Fprintf(&out, "0x%02x=%d", op, counts[op])
	}
	return out.String()
}

// setAccount labels c's backlog abort log with the account its client
// authenticated as.
func (c *Conn) setAccount(name string) {
	c.mu.Lock()
	c.account = name
	c.mu.Unlock()
}

// ObserveSends installs fn to see every frame a Session queues on c: its
// cleartext payload, length header stripped, on the sending goroutine, before
// the frame is encrypted and queued. It is a test seam for which goroutine
// queued a reply — the writer goroutine owns the socket write, so nothing on
// the wire tells. fn must not retain payload or send on c. Call it before c
// reaches its handler; unset, a send pays only a nil check.
func (c *Conn) ObserveSends(fn func(payload []byte)) {
	c.observeSend = fn
}

// ObserveReads installs fn to run on the reading goroutine each time a
// Session on c starts waiting for the next inbound frame, which is when
// every earlier frame has been fully handled and the wait's read deadline is
// set. fn gets how long that wait may last. It is a test seam for knowing
// the server has caught up with a client. Call it before c reaches its
// handler; unset, a read pays only a nil check.
func (c *Conn) ObserveReads(fn func(timeout time.Duration)) {
	c.observeRead = fn
}

// ObservePersistWaits installs fn to run on the reading goroutine each time
// the handler of a frame read from c starts waiting for the saves queued for
// owners (every owner when owners is empty); the func fn returns runs once
// that wait ends. It is a test seam for knowing a frame's handler is parked
// on the persistence worker rather than still working. fn must not retain
// owners. Call it before c reaches its handler; unset, a wait pays only a nil
// check.
func (c *Conn) ObservePersistWaits(fn func(owners []int32) (done func())) {
	c.observePersistWait = fn
}

// Stopped returns a channel that is closed once the writer has exited and
// the underlying connection is closed, so the peer reads the close. It is a
// test seam for telling a connection that has queued its last frame from one
// whose close is on the wire.
func (c *Conn) Stopped() <-chan struct{} { return c.stopped }

func (c *Conn) signal() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *Conn) stop() {
	c.stopOnce.Do(func() { close(c.stopping) })
}

// Close stops accepting new sends, flushes any already queued, then
// closes the underlying connection. Safe to call more than once; every
// call blocks until the underlying connection is actually closed.
func (c *Conn) Close() error {
	c.closeLater()
	<-c.stopped
	return c.closeErr
}

// closeLater is Close without the wait: the writer flushes what is queued
// and closes the underlying connection on its own, which for a peer that
// stopped reading happens only once the write deadline gives up on it.
func (c *Conn) closeLater() {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	c.signal()
}
