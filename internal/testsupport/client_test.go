package testsupport

import (
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

// TestReadFinishesAFrameWhoseDeadlinePassesMidFrame pins that a read never
// splits a frame: once a frame's header has arrived, its payload is read to
// the end even when the caller's wait runs out first. Reporting a timeout
// there would leave the header consumed and every later read misaligned.
func TestReadFinishesAFrameWhoseDeadlinePassesMidFrame(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { clientConn.Close() })
	t.Cleanup(func() { serverConn.Close() })
	c := &ScriptedClient{t: t, conn: clientConn, handshaken: true}

	frame, err := wire.FrameBytes([]byte{0x0d, 0x01, 0x02})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		serverConn.Write(frame[:wire.FrameHeaderSize])
		time.Sleep(100 * time.Millisecond)
		serverConn.Write(frame[wire.FrameHeaderSize:])
	}()

	got := c.ReadWithTimeout(20 * time.Millisecond)
	if string(got) != string(frame[wire.FrameHeaderSize:]) {
		t.Fatalf("read = %x, want the whole payload %x", got, frame[wire.FrameHeaderSize:])
	}
}
