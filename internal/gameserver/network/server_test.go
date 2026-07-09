package network

import (
	"bufio"
	"context"
	"io"
	"net"
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

type testAddr string

func (a testAddr) Network() string { return string(a) }
func (a testAddr) String() string  { return string(a) }
