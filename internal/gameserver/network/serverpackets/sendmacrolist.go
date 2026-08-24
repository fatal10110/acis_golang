package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeSendMacroList is the wire opcode for SendMacroList.
const OpcodeSendMacroList = 0xe7

// FrameSendMacroListEmpty builds the empty macro-list packet sent on world entry.
func FrameSendMacroListEmpty() wire.Frame {
	w := newFrameWriter(OpcodeSendMacroList)
	w.WriteInt32(0) // macro revision
	w.WriteUint8(0) // unknown
	w.WriteUint8(0) // macro count
	w.WriteUint8(0) // no macro follows
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
