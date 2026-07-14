package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// FrameExMailArrived builds the mail-popup notification packet.
func FrameExMailArrived() wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExMailArrived)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
