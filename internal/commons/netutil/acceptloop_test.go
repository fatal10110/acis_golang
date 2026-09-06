package netutil

import (
	"bytes"
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/rs/zerolog"
)

// setTCPNoDelayForTest reads the TCP_NODELAY socket option directly so the
// test observes the real socket state rather than trusting a setter.
func setTCPNoDelayForTest(fd uintptr) (bool, error) {
	var v int32
	vLen := int32(4)
	_, _, errno := syscall.Syscall6(
		syscall.SYS_GETSOCKOPT,
		fd,
		uintptr(syscall.IPPROTO_TCP),
		uintptr(syscall.TCP_NODELAY),
		uintptr(unsafe.Pointer(&v)),
		uintptr(unsafe.Pointer(&vLen)),
		0,
	)
	if errno != 0 {
		return false, errno
	}
	return v != 0, nil
}

func TestAcceptLoopDrainsHandlersBeforeReturningClosedError(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() { client.Close() })

	ln := &oneConnThenErrorListener{conn: server, err: net.ErrClosed, addr: server.LocalAddr()}
	finished := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- AcceptLoop(context.Background(), ln, func(conn net.Conn) {
			buf := make([]byte, 1)
			if _, err := conn.Read(buf); err == nil {
				t.Error("handler Read returned nil after accept failure")
			}
			close(finished)
		}, zerolog.Nop())
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("AcceptLoop error = %v, want %v", err, net.ErrClosed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("AcceptLoop did not return after accept failure")
	}

	select {
	case <-finished:
	default:
		t.Fatal("AcceptLoop returned before connection handler finished")
	}
}

func TestAcceptLoopRetriesTemporaryAcceptError(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() { client.Close() })

	ln := &temporaryErrorThenConnListener{
		conn: server,
		err:  net.ErrClosed,
		addr: server.LocalAddr(),
	}
	handled := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- AcceptLoop(context.Background(), ln, func(conn net.Conn) {
			conn.Close()
			close(handled)
		}, zerolog.Nop())
	}()

	select {
	case <-handled:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not run after temporary accept error")
	}
	if err := <-errCh; !errors.Is(err, net.ErrClosed) {
		t.Fatalf("AcceptLoop error = %v, want %v", err, net.ErrClosed)
	}
}

func TestAcceptLoopReturnsImmediatelyOnErrClosed(t *testing.T) {
	ln := &oneConnThenErrorListener{err: net.ErrClosed}
	start := time.Now()
	err := AcceptLoop(context.Background(), ln, func(net.Conn) {}, zerolog.Nop())
	if !errors.Is(err, net.ErrClosed) {
		t.Fatalf("AcceptLoop error = %v, want %v", err, net.ErrClosed)
	}
	if elapsed := time.Since(start); elapsed >= acceptRetryMin {
		t.Fatalf("AcceptLoop returned after %v, want immediately on net.ErrClosed", elapsed)
	}
}

func TestAcceptLoopBacksOffOnAcceptError(t *testing.T) {
	var mu sync.Mutex
	var calls []time.Time
	ln := &recordingErrorListener{
		accept: func() (net.Conn, error) {
			mu.Lock()
			calls = append(calls, time.Now())
			n := len(calls)
			mu.Unlock()
			if n >= 3 {
				return nil, net.ErrClosed
			}
			return nil, errors.New("accept busy")
		},
	}

	err := AcceptLoop(context.Background(), ln, func(net.Conn) {}, zerolog.Nop())
	if !errors.Is(err, net.ErrClosed) {
		t.Fatalf("AcceptLoop error = %v, want %v", err, net.ErrClosed)
	}

	mu.Lock()
	got := append([]time.Time(nil), calls...)
	mu.Unlock()
	if len(got) != 3 {
		t.Fatalf("Accept calls = %d, want 3", len(got))
	}
	if delay := got[1].Sub(got[0]); delay < 4*time.Millisecond {
		t.Fatalf("first retry delay = %v, want ~5ms backoff", delay)
	}
	if delay := got[2].Sub(got[1]); delay < 8*time.Millisecond {
		t.Fatalf("second retry delay = %v, want ~10ms backoff", delay)
	}
}

func TestAcceptLoopSetsTCPNoDelayOnAcceptedConnections(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	noDelay := make(chan bool, 1)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	errCh := make(chan error, 1)
	go func() {
		errCh <- AcceptLoop(ctx, ln, func(conn net.Conn) {
			tcp, ok := conn.(*net.TCPConn)
			if !ok {
				t.Error("accepted connection is not a *net.TCPConn")
				noDelay <- false
				return
			}
			raw, err := tcp.SyscallConn()
			if err != nil {
				t.Errorf("SyscallConn: %v", err)
				noDelay <- false
				return
			}
			var got bool
			var ctlErr error
			if err := raw.Control(func(fd uintptr) {
				got, ctlErr = setTCPNoDelayForTest(fd)
			}); err != nil {
				t.Errorf("Control: %v", err)
				noDelay <- false
				return
			}
			if ctlErr != nil {
				t.Errorf("getsockopt TCP_NODELAY: %v", ctlErr)
				noDelay <- false
				return
			}
			noDelay <- got
			conn.Close()
		}, zerolog.Nop())
	}()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	select {
	case got := <-noDelay:
		if !got {
			t.Fatal("accepted connection does not have TCP_NODELAY enabled")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("handler never ran")
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("AcceptLoop error = %v, want nil on cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("AcceptLoop did not return after cancellation")
	}
}

type oneConnThenErrorListener struct {
	conn net.Conn
	err  error
	addr net.Addr
}

func (l *oneConnThenErrorListener) Accept() (net.Conn, error) {
	if l.conn != nil {
		conn := l.conn
		l.conn = nil
		return conn, nil
	}
	return nil, l.err
}

func (*oneConnThenErrorListener) Close() error     { return nil }
func (l *oneConnThenErrorListener) Addr() net.Addr { return l.addr }

type temporaryErrorThenConnListener struct {
	conn net.Conn
	err  error
	addr net.Addr
	step int
}

func (l *temporaryErrorThenConnListener) Accept() (net.Conn, error) {
	switch l.step {
	case 0:
		l.step++
		return nil, temporaryError{l.err}
	case 1:
		l.step++
		return l.conn, nil
	default:
		return nil, l.err
	}
}

func (*temporaryErrorThenConnListener) Close() error     { return nil }
func (l *temporaryErrorThenConnListener) Addr() net.Addr { return l.addr }

type recordingErrorListener struct {
	accept func() (net.Conn, error)
}

func (l *recordingErrorListener) Accept() (net.Conn, error) { return l.accept() }
func (*recordingErrorListener) Close() error                { return nil }
func (*recordingErrorListener) Addr() net.Addr              { return nil }

type temporaryError struct{ error }

func (temporaryError) Temporary() bool { return true }
func (temporaryError) Timeout() bool   { return false }

// TestAcceptLoopLogsAndRetriesPermanentAcceptError pins the two properties
// the backoff rewrite left uncovered: a listener that only ever returns a
// non-net.ErrClosed error is retried indefinitely rather than returning,
// and every retry is reported through the logger. Without the Warn call in
// AcceptLoop a wedged listener leaves the log file completely empty.
func TestAcceptLoopLogsAndRetriesPermanentAcceptError(t *testing.T) {
	var logMu sync.Mutex
	var logged bytes.Buffer
	log := zerolog.New(&lockedWriter{mu: &logMu, w: &logged})

	acceptErr := errors.New("accept wedged")
	var mu sync.Mutex
	calls := 0
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ln := &recordingErrorListener{
		accept: func() (net.Conn, error) {
			mu.Lock()
			calls++
			n := calls
			mu.Unlock()
			if n >= 3 {
				cancel()
			}
			return nil, acceptErr
		},
	}

	errCh := make(chan error, 1)
	go func() { errCh <- AcceptLoop(ctx, ln, func(net.Conn) {}, log) }()

	select {
	case err := <-errCh:
		// The permanent error must never propagate: only ctx cancellation
		// (nil) or net.ErrClosed ends the loop.
		if err != nil {
			t.Fatalf("AcceptLoop error = %v, want nil after ctx cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("AcceptLoop did not return after ctx cancellation")
	}

	mu.Lock()
	got := calls
	mu.Unlock()
	if got < 3 {
		t.Fatalf("Accept calls = %d, want >= 3 (error must be retried, not returned)", got)
	}

	logMu.Lock()
	out := logged.String()
	logMu.Unlock()
	if !strings.Contains(out, acceptErr.Error()) {
		t.Fatalf("accept error %q not logged; log = %q", acceptErr, out)
	}
	if !strings.Contains(out, "retry_in") {
		t.Fatalf("retry delay not logged; log = %q", out)
	}
}

type lockedWriter struct {
	mu *sync.Mutex
	w  *bytes.Buffer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
