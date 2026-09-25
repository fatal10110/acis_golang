package testsupport

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

func TestSyncBarrierFramesReturnsPrecedingFramesInOrder(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { clientConn.Close() })
	t.Cleanup(func() { serverConn.Close() })
	c := &ScriptedClient{t: t, conn: clientConn, handshaken: true}

	errCh := make(chan error, 1)
	go func() {
		if _, err := wire.ReadFrame(serverConn); err != nil {
			errCh <- err
			return
		}
		for _, frame := range [][]byte{{0x10}, {0x11}, {0x12}} {
			if err := wire.WriteFrame(serverConn, frame); err != nil {
				errCh <- err
				return
			}
		}
		errCh <- nil
	}()

	frames := SyncBarrierFrames(t, c, func() { c.Send([]byte{0x01}) }, 0x12)
	if len(frames) != 2 || frames[0][0] != 0x10 || frames[1][0] != 0x11 {
		t.Fatalf("pre-barrier frames = %x, want [[10] [11]]", frames)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("barrier server: %v", err)
	}
}

// TestAwaitCloseSpendsOneBudgetOnTheAwaitClock pins that AwaitClose's d is
// one budget on the clock await advances, shared by every frame it drains:
// a server that keeps sending a frame every second and never closes must
// fail AwaitClose(3s) after about three frames, not restart the budget on
// each frame.
func TestAwaitCloseSpendsOneBudgetOnTheAwaitClock(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { clientConn.Close() })
	t.Cleanup(func() { serverConn.Close() })
	c := &ScriptedClient{t: t, conn: clientConn, handshaken: true}

	var now time.Time
	frames := 0
	c.SetAwait(func(d time.Duration) bool {
		if d < time.Second {
			now = now.Add(max(d, 0))
			return false
		}
		now = now.Add(time.Second)
		frames++
		go wire.WriteFrame(serverConn, []byte{0x01})
		return true
	}, func() time.Time { return now })

	if c.AwaitClose(3 * time.Second) {
		t.Fatal("AwaitClose reported a close the server never made")
	}
	if frames > 3 {
		t.Fatalf("AwaitClose drained %d frames in a 3s budget of 1s frames, want at most 3", frames)
	}
}

// TestExpectClosedWaitsOnTheAwaitClock pins that ExpectClosed waits through
// await, as every read does, so on a driven clock it waits for the server's
// progress rather than a raw socket deadline, and that a server which never
// closes still fails it once await gives up.
func TestExpectClosedWaitsOnTheAwaitClock(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { clientConn.Close() })
	c := &ScriptedClient{t: t, conn: clientConn, handshaken: true}

	var now time.Time
	awaited := 0
	c.SetAwait(func(d time.Duration) bool {
		awaited++
		now = now.Add(d)
		return false
	}, func() time.Time { return now })
	if err := c.closed(5 * time.Second); err == nil {
		t.Fatal("closed reported a close the server never made")
	}
	if awaited != 1 {
		t.Fatalf("await calls = %d, want 1", awaited)
	}

	c.SetAwait(func(time.Duration) bool {
		serverConn.Close()
		return true
	}, func() time.Time { return now })
	if err := c.closed(5 * time.Second); err != nil {
		t.Fatalf("closed after the server closed = %v, want nil", err)
	}
}

// TestExpectClosedFailsOnAPartialFrame pins that bytes before the close are
// not a clean close: a frame cut off by the close, or stalled mid-frame past
// its read bound, fails ExpectClosed like a whole frame or a timeout does.
func TestExpectClosedFailsOnAPartialFrame(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { clientConn.Close() })
	c := &ScriptedClient{t: t, conn: clientConn, handshaken: true}
	go func() {
		serverConn.Write([]byte{5})
		serverConn.Close()
	}()
	if err := c.closed(time.Second); err == nil {
		t.Fatal("closed reported a clean close after a partial frame")
	}
}

// TestReadFinishesAFrameWhoseDeadlinePassesMidFrame pins that a read never
// splits a frame: once a frame's header has arrived, its payload is read to
// the end even when the caller's wait runs out first. Reporting a timeout
// there would leave the header consumed and every later read misaligned.
func TestReadFinishesAFrameWhoseDeadlinePassesMidFrame(t *testing.T) {
	c, serverConn := tcpClient(t)
	frame, err := wire.FrameBytes([]byte{0x0d, 0x01, 0x02})
	if err != nil {
		t.Fatal(err)
	}
	// The header is already buffered when the read starts; only the
	// payload arrives after the read's deadline.
	if _, err := serverConn.Write(frame[:wire.FrameHeaderSize]); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(100 * time.Millisecond)
		serverConn.Write(frame[wire.FrameHeaderSize:])
	}()

	got := c.ReadWithTimeout(20 * time.Millisecond)
	if string(got) != string(frame[wire.FrameHeaderSize:]) {
		t.Fatalf("read = %x, want the whole payload %x", got, frame[wire.FrameHeaderSize:])
	}
}

// TestTryReadReportsACutFrameAsAnError pins that a frame which stalls after
// its first bytes is an error, never the nil, nil a timeout reads as, and
// that no inspection of the error finds a timeout in it. It waits out
// frameInFlight.
func TestTryReadReportsACutFrameAsAnError(t *testing.T) {
	t.Parallel()
	c, serverConn := tcpClient(t)
	frame, err := wire.FrameBytes([]byte{0x0d, 0x01, 0x02})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := serverConn.Write(frame[:wire.FrameHeaderSize+1]); err != nil {
		t.Fatal(err)
	}

	got, err := c.TryRead(20 * time.Millisecond)
	if err == nil {
		t.Fatalf("TryRead = %x, nil; want an error for the cut frame", got)
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		t.Fatalf("TryRead error %v unwraps to a timeout", err)
	}
}

// tcpClient returns a ScriptedClient on a loopback TCP connection and the
// server's end of it. Unlike net.Pipe, TCP buffers a write, so a test can
// put bytes on the wire before the read under test starts.
func tcpClient(t *testing.T) (*ScriptedClient, net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	c := Dial(t, ln.Addr().String())
	c.handshaken = true
	serverConn, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { serverConn.Close() })
	return c, serverConn
}
