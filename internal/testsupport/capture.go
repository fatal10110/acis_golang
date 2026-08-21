package testsupport

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// FrameCapture records raw frame payloads (opcode byte first, length prefix
// stripped) delivered through a frame sender. It doubles as a send hook for
// tests that attach it directly to actors instead of driving a socket.
type FrameCapture struct {
	mu     sync.Mutex
	frames [][]byte
}

// Send implements the frame-sender contract: it consumes the frame and
// records its payload without the 2-byte length prefix.
func (c *FrameCapture) Send(frame wire.Frame) bool {
	defer frame.Release()
	raw := frame.Bytes()
	payload := make([]byte, len(raw)-2)
	copy(payload, raw[2:])
	c.mu.Lock()
	c.frames = append(c.frames, payload)
	c.mu.Unlock()
	return true
}

// Frames returns a safe copy of the recorded payload frames. Tests that may
// race a background goroutine still delivering frames (e.g. an in-flight
// move-then-arrive callback) must use this instead of assuming the slice is
// stable.
func (c *FrameCapture) Frames() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([][]byte(nil), c.frames...)
}

// ResetCapture clears every capture's recorded frames.
func ResetCapture(captures ...*FrameCapture) {
	for _, capture := range captures {
		capture.mu.Lock()
		capture.frames = nil
		capture.mu.Unlock()
	}
}

// FrameOpcodes extracts the leading opcode byte of every recorded frame.
func FrameOpcodes(frames [][]byte) []byte {
	out := make([]byte, 0, len(frames))
	for _, frame := range frames {
		if len(frame) > 0 {
			out = append(out, frame[0])
		}
	}
	return out
}

// AssertOpcodeSequence checks that frames carry exactly the given opcodes,
// in order.
func AssertOpcodeSequence(t *testing.T, frames [][]byte, want ...byte) {
	t.Helper()
	got := FrameOpcodes(frames)
	if string(got) != string(want) {
		t.Fatalf("opcodes = %x, want %x", got, want)
	}
}
