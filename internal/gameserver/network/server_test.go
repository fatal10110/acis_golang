package network

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	return ln
}

func TestServeEchoesThroughSend(t *testing.T) {
	ln := listen(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- Serve(ctx, ln, func(ctx context.Context, conn *Conn) {
			buf := make([]byte, 5)
			if _, err := conn.Read(buf); err != nil {
				return
			}
			conn.Send(buf)
		}, nil)
	}()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	if _, err := client.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}

	client.SetReadDeadline(time.Now().Add(5 * time.Second))
	got := make([]byte, 5)
	if _, err := bufio.NewReader(client).Read(got); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("echo = %q, want %q", got, "hello")
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("Serve returned error after cancel: %v", err)
	}
}

func TestServeStopsOnContextCancel(t *testing.T) {
	ln := listen(t)
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- Serve(ctx, ln, func(context.Context, *Conn) {}, nil)
	}()

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Serve returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after context cancel")
	}

	if _, err := net.Dial("tcp", ln.Addr().String()); err == nil {
		t.Fatal("listener still accepting connections after cancel")
	}
}

func TestServeHandlesConnectionsConcurrently(t *testing.T) {
	ln := listen(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const clients = 8
	handled := make(chan struct{}, clients)

	go Serve(ctx, ln, func(ctx context.Context, conn *Conn) {
		handled <- struct{}{}
		<-ctx.Done()
	}, nil)

	conns := make([]net.Conn, clients)
	for i := 0; i < clients; i++ {
		c, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		conns[i] = c
	}
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	for i := 0; i < clients; i++ {
		select {
		case <-handled:
		case <-time.After(5 * time.Second):
			t.Fatalf("only %d/%d connections handled", i, clients)
		}
	}
}

func TestConnSendAfterCloseReturnsFalse(t *testing.T) {
	ln := listen(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	closed := make(chan *Conn, 1)
	go Serve(ctx, ln, func(ctx context.Context, conn *Conn) {
		conn.Close()
		closed <- conn
	}, nil)

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	var conn *Conn
	select {
	case conn = <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("connection was not handled")
	}

	if conn.Send([]byte("late")) {
		t.Fatal("Send on closed connection returned true, want false")
	}
}

func TestConnSendFrameAfterCloseReleasesFrame(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()

	conn := newConn(server, nil)
	if err := conn.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	released := make(chan struct{}, 1)
	frame := wire.OwnedFrame([]byte{0x02, 0x00}, nil, func(*wire.Writer) { released <- struct{}{} })
	if conn.SendFrame(frame) {
		t.Fatal("SendFrame on closed connection returned true, want false")
	}
	select {
	case <-released:
	case <-time.After(5 * time.Second):
		t.Fatal("owned frame was not released after rejected send")
	}
}

func TestConnWriteLoopExitsOnWriteErrorAndCloseCompletes(t *testing.T) {
	server, client := net.Pipe()
	conn := newConn(server, nil)

	// Closing the read side makes the next write on server fail
	// synchronously, exercising writeLoop's error path.
	client.Close()
	conn.Send([]byte("payload"))

	done := make(chan struct{})
	go func() {
		conn.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not complete after writeLoop exited on a write error")
	}
}

func TestConnSendFrameAfterWriteLoopExitReturnsFalseAndReleasesFrame(t *testing.T) {
	server, client := net.Pipe()
	conn := newConn(server, nil)

	client.Close()
	conn.Send([]byte("payload"))

	select {
	case <-conn.stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("writeLoop did not stop after write error")
	}

	released := make(chan struct{}, 1)
	frame := wire.OwnedFrame([]byte{0x02, 0x00}, nil, func(*wire.Writer) { released <- struct{}{} })
	if conn.SendFrame(frame) {
		t.Fatal("SendFrame after writeLoop exit returned true, want false")
	}

	select {
	case <-released:
	case <-time.After(5 * time.Second):
		t.Fatal("owned frame was not released after stopped writer rejected send")
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestConnReleasesAlreadyQueuedFramesAfterWriteError(t *testing.T) {
	raw := &blockingErrorConn{
		writeStarted: make(chan struct{}),
		fail:         make(chan struct{}),
	}
	conn := newConn(raw, nil)

	firstReleased := make(chan struct{}, 1)
	first := wire.OwnedFrame([]byte{0x02, 0x00}, nil, func(*wire.Writer) { firstReleased <- struct{}{} })
	if !conn.SendFrame(first) {
		t.Fatal("first SendFrame returned false")
	}

	select {
	case <-raw.writeStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("write did not start")
	}

	secondReleased := make(chan struct{}, 1)
	second := wire.OwnedFrame([]byte{0x02, 0x00}, nil, func(*wire.Writer) { secondReleased <- struct{}{} })
	if !conn.SendFrame(second) {
		t.Fatal("second SendFrame returned false")
	}

	close(raw.fail)

	select {
	case <-firstReleased:
	case <-time.After(5 * time.Second):
		t.Fatal("failed write frame was not released")
	}
	select {
	case <-conn.stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("writeLoop did not stop after write error")
	}
	select {
	case <-secondReleased:
	case <-time.After(5 * time.Second):
		t.Fatal("already queued frame was not released after write error")
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestConnDrainBatchCoalescesBurst is the drainBatch analogue of the
// issue's acceptance test ("N queued payloads -> 1 vectored write"): it
// checks the part writeLoop actually controls, that a burst already
// sitting in c.out is collected into one ordered batch instead of being
// handed to writeBatch one frame at a time.
func TestConnDrainBatchCoalescesBurst(t *testing.T) {
	conn := &Conn{
		out:      make(chan queuedWrite, outboundBuffer),
		stopping: make(chan struct{}),
		stopped:  make(chan struct{}),
	}

	const n = 5
	for i := 0; i < n; i++ {
		if !conn.send(queuedWrite{frame: wire.BorrowedFrame([]byte{byte(i)})}) {
			t.Fatalf("send %d failed", i)
		}
	}

	first := <-conn.out
	batch := conn.drainBatch(nil, first)
	if len(batch) != n {
		t.Fatalf("batch len = %d, want %d", len(batch), n)
	}
	for i, queued := range batch {
		if queued.frame.Bytes()[0] != byte(i) {
			t.Fatalf("batch[%d] = %v, want order preserved", i, queued.frame.Bytes())
		}
	}
}

// TestConnDrainBatchBoundedByOutboundBuffer checks the batch cap the
// issue calls for ("bound the batch...so one slow reader can't build an
// unbounded batch"): even with more already queued, drainBatch stops at
// outboundBuffer and leaves the rest in the channel.
func TestConnDrainBatchBoundedByOutboundBuffer(t *testing.T) {
	conn := &Conn{
		out:      make(chan queuedWrite, outboundBuffer+10),
		stopping: make(chan struct{}),
		stopped:  make(chan struct{}),
	}

	for i := 0; i < outboundBuffer+10; i++ {
		conn.out <- queuedWrite{frame: wire.BorrowedFrame([]byte{byte(i)})}
	}

	first := <-conn.out
	batch := conn.drainBatch(nil, first)
	if len(batch) != outboundBuffer {
		t.Fatalf("batch len = %d, want %d (bounded)", len(batch), outboundBuffer)
	}
	if remaining := len(conn.out); remaining != 10 {
		t.Fatalf("remaining queued = %d, want 10 left undrained", remaining)
	}
}

// TestConnWriteBatchReleasesAllOnError covers the issue's pool-return
// requirement on failure: net.Buffers.WriteTo stops at the first write
// error and never attempts the rest of the batch, but every frame in
// the batch - written or not - must still be released back to the pool.
func TestConnWriteBatchReleasesAllOnError(t *testing.T) {
	conn := &Conn{Conn: alwaysFailConn{}}

	var released [2]int32
	batch := []queuedWrite{
		{frame: wire.OwnedFrame([]byte("a"), nil, func(*wire.Writer) { atomic.AddInt32(&released[0], 1) })},
		{frame: wire.OwnedFrame([]byte("b"), nil, func(*wire.Writer) { atomic.AddInt32(&released[1], 1) })},
	}

	if _, err := conn.writeBatch(batch, nil); err == nil {
		t.Fatal("writeBatch returned nil error, want write failure")
	}
	if atomic.LoadInt32(&released[0]) != 1 || atomic.LoadInt32(&released[1]) != 1 {
		t.Fatalf("released = %v, want both released once", released)
	}
}

// TestConnBurstArrivesInOrderAndReleases exercises the real writeLoop
// goroutine (not just drainBatch/writeBatch in isolation) with a burst
// of sends, confirming the byte stream the peer sees is unchanged and
// every frame is eventually released, regardless of how the burst gets
// split into batches.
func TestConnBurstArrivesInOrderAndReleases(t *testing.T) {
	server, client := net.Pipe()
	conn := newConn(server, nil)
	defer client.Close()

	const want = "abcde"
	var released int32
	for i := 0; i < len(want); i++ {
		b := want[i]
		frame := wire.OwnedFrame([]byte{b}, nil, func(*wire.Writer) { atomic.AddInt32(&released, 1) })
		if !conn.SendFrame(frame) {
			t.Fatalf("SendFrame %q failed", b)
		}
	}

	got := make([]byte, len(want))
	if _, err := io.ReadFull(client, got); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if atomic.LoadInt32(&released) != int32(len(want)) {
		t.Fatalf("released = %d, want %d", released, len(want))
	}
}

// TestConnCloseFlushesBurst confirms Close's documented contract still
// holds once writes are batched: a burst queued right before Close is
// still delivered in full, not dropped by the batching change.
func TestConnCloseFlushesBurst(t *testing.T) {
	ln := listen(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const want = "flush-me"
	go Serve(ctx, ln, func(ctx context.Context, conn *Conn) {
		for i := 0; i < len(want); i++ {
			conn.Send([]byte{want[i]})
		}
		conn.Close()
	}, nil)

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	client.SetReadDeadline(time.Now().Add(5 * time.Second))
	got := make([]byte, len(want))
	if _, err := io.ReadFull(client, got); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestServeSurvivesHandlerPanic(t *testing.T) {
	ln := listen(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- Serve(ctx, ln, func(ctx context.Context, conn *Conn) {
			panic("boom")
		}, nil)
	}()

	bad, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer bad.Close()

	// The panicking connection must be closed (proving the deferred
	// recover ran) without Serve itself returning/crashing.
	bad.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := bad.Read(make([]byte, 1)); err == nil {
		t.Fatal("expected panicking connection to be closed")
	}

	// Serve must still be accepting for other clients.
	good, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial after panic: %v", err)
	}
	defer good.Close()

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Serve returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after context cancel")
	}
}

type blockingErrorConn struct {
	writeStarted chan struct{}
	fail         chan struct{}
}

func (c *blockingErrorConn) Read([]byte) (int, error) { return 0, io.EOF }
func (c *blockingErrorConn) Write([]byte) (int, error) {
	close(c.writeStarted)
	<-c.fail
	return 0, io.ErrClosedPipe
}
func (c *blockingErrorConn) Close() error                     { return nil }
func (c *blockingErrorConn) LocalAddr() net.Addr              { return testAddr("local") }
func (c *blockingErrorConn) RemoteAddr() net.Addr             { return testAddr("remote") }
func (c *blockingErrorConn) SetDeadline(time.Time) error      { return nil }
func (c *blockingErrorConn) SetReadDeadline(time.Time) error  { return nil }
func (c *blockingErrorConn) SetWriteDeadline(time.Time) error { return nil }

// alwaysFailConn is a net.Conn stub whose every Write fails, used to
// verify writeBatch's release-on-error behavior without depending on
// net.Buffers' internal dispatch to a real *net.TCPConn.
type alwaysFailConn struct{}

func (alwaysFailConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (alwaysFailConn) Write([]byte) (int, error)        { return 0, errors.New("write failed") }
func (alwaysFailConn) Close() error                     { return nil }
func (alwaysFailConn) LocalAddr() net.Addr              { return testAddr("local") }
func (alwaysFailConn) RemoteAddr() net.Addr             { return testAddr("remote") }
func (alwaysFailConn) SetDeadline(time.Time) error      { return nil }
func (alwaysFailConn) SetReadDeadline(time.Time) error  { return nil }
func (alwaysFailConn) SetWriteDeadline(time.Time) error { return nil }

type testAddr string

func (a testAddr) Network() string { return string(a) }
func (a testAddr) String() string  { return string(a) }
