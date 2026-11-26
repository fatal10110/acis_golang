package network

import (
	"errors"
	"io"
	"net"
	"os"
	"syscall"
	"testing"

	"github.com/rs/zerolog"
)

func socketErr(op string, errno syscall.Errno) error {
	return &net.OpError{Op: op, Net: "tcp", Err: os.NewSyscallError(op, errno)}
}

// A peer that resets or aborts the connection hung up, the same as one that
// closed it; every other read error keeps its own classification.
func TestReadFrameErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		name           string
		err            error
		hangUp, normal bool
	}{
		{"EOF", io.EOF, true, true},
		{"mid-frame EOF", io.ErrUnexpectedEOF, true, true},
		{"reset", socketErr("read", syscall.ECONNRESET), true, true},
		{"aborted", socketErr("read", syscall.ECONNABORTED), true, true},
		{"bare reset", syscall.ECONNRESET, true, true},
		{"closed here", net.ErrClosed, false, true},
		{"idle deadline", os.ErrDeadlineExceeded, false, true},
		{"broken pipe", socketErr("read", syscall.EPIPE), false, false},
		{"other", errors.New("decrypt failed"), false, false},
	} {
		if got := hangUp(tc.err); got != tc.hangUp {
			t.Errorf("hangUp(%s) = %t, want %t", tc.name, got, tc.hangUp)
		}
		if got := normalReadFrameError(tc.err); got != tc.normal {
			t.Errorf("normalReadFrameError(%s) = %t, want %t", tc.name, got, tc.normal)
		}
	}
}

// failingWriteConn fails every write with err.
type failingWriteConn struct {
	net.Conn
	err error
}

func (c failingWriteConn) Write([]byte) (int, error) { return 0, c.err }

// A write to a peer that already left logs below warn; any other write
// failure stays at warn.
func TestConnWriteFailureLogLevel(t *testing.T) {
	for _, tc := range []struct {
		name  string
		err   error
		level string
	}{
		{"broken pipe", socketErr("writev", syscall.EPIPE), "debug"},
		{"reset", socketErr("writev", syscall.ECONNRESET), "debug"},
		{"timeout", &net.OpError{Op: "writev", Net: "tcp", Err: os.ErrDeadlineExceeded}, "warn"},
		{"unknown", errors.New("unknown write failure"), "warn"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, client := net.Pipe()
			t.Cleanup(func() { _ = client.Close() })
			var logs lockedBuffer
			c := newConn(failingWriteConn{Conn: server, err: tc.err}, zerolog.New(&logs), testRejects())
			sendWithin(t, c, pooledFrame(0x03, 16, 64))
			awaitStopped(t, c)
			for _, entry := range logs.lines(t) {
				if entry["message"] == "game connection write failed, closing" {
					if entry["level"] != tc.level {
						t.Fatalf("write failure logged at %v, want %s: %v", entry["level"], tc.level, entry)
					}
					return
				}
			}
			t.Fatalf("write failure not logged; logs: %v", logs.lines(t))
		})
	}
}
