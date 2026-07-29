package netutil

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

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
