package netutil

import (
	"context"
	"errors"
	"net"
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

func TestAcceptLoopDrainsHandlersBeforeReturningAcceptError(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() { client.Close() })

	wantErr := errors.New("accept failed")
	ln := &oneConnThenErrorListener{conn: server, err: wantErr, addr: server.LocalAddr()}
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
		if !errors.Is(err, wantErr) {
			t.Fatalf("AcceptLoop error = %v, want %v", err, wantErr)
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
