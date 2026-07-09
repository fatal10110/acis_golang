package serverpackets

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

const packetWriterCapacity = 256

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

func releaseFrameWriter(w *wire.Writer) {
	packetWriterPool.Put(w)
}
