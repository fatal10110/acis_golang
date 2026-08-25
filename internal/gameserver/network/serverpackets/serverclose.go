package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeServerClose is the wire opcode for ServerClose, the empty frame a
// game client receives immediately before its connection is closed.
const OpcodeServerClose = 0x26

// FrameServerClose builds the ServerClose packet as an owned frame.
func FrameServerClose() wire.Frame {
	w := newFrameWriter(OpcodeServerClose)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
