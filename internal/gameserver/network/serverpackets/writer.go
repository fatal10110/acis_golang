package serverpackets

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

const (
	packetWriterCapacity = 256
	// maxPacketWriterCapacity keeps routine variable-length packets pooled while
	// preventing outliers from pinning buffers near the uint16 frame-size limit.
	maxPacketWriterCapacity = 8 * 1024
)

var packetWriterPool = sync.Pool{
	New: func() any {
		return wire.NewFrameWriter(packetWriterCapacity)
	},
}

// newWriter starts a game server packet with its opcode byte.
func newWriter(opcode byte) *wire.Writer {
	return wire.NewPacketWriter(opcode)
}

func newFrameWriter(opcode byte) *wire.Writer {
	w := packetWriterPool.Get().(*wire.Writer)
	w.ResetFrame(packetWriterCapacity)
	w.WriteUint8(opcode)
	return w
}

func poolable(w *wire.Writer) bool {
	return w.Cap() <= maxPacketWriterCapacity
}

func releaseFrameWriter(w *wire.Writer) {
	if !poolable(w) {
		return
	}
	packetWriterPool.Put(w)
}

// CopyFrame returns an independently owned pooled copy of frame for a session
// that encrypts its outgoing bytes in place. It reports false when frame does
// not contain a complete header.
func CopyFrame(frame wire.Frame) (wire.Frame, bool) {
	return wire.CopyFrame(frame)
}
