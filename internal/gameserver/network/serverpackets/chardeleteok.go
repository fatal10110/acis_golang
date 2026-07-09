package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeCharDeleteOk is the wire opcode for CharDeleteOk, acknowledging a
// successful character deletion request.
const OpcodeCharDeleteOk = 0x23

// EncodeCharDeleteOk builds the CharDeleteOk packet.
func EncodeCharDeleteOk() []byte {
	return newWriter(OpcodeCharDeleteOk).Bytes()
}

// FrameCharDeleteOk builds the CharDeleteOk packet as an owned frame.
func FrameCharDeleteOk() wire.Frame {
	w := newFrameWriter(OpcodeCharDeleteOk)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
