package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeCharCreateOk is the wire opcode for CharCreateOk, acknowledging a
// successful character creation.
const OpcodeCharCreateOk = 0x19

// EncodeCharCreateOk builds the CharCreateOk packet.
func EncodeCharCreateOk() []byte {
	w := newWriter(OpcodeCharCreateOk)
	w.WriteInt32(1)
	return w.Bytes()
}

// FrameCharCreateOk builds the CharCreateOk packet as an owned frame.
func FrameCharCreateOk() wire.Frame {
	w := newFrameWriter(OpcodeCharCreateOk)
	w.WriteInt32(1)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
