package testsupport

import (
	"net"
	"testing"

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
