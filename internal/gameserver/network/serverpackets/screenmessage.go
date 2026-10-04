package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeExShowScreenMessage is the extended opcode of an on-screen
// message.
const OpcodeExShowScreenMessage uint16 = 0x0038

// screenMessagePositionTopCenter is the on-screen message position a
// plain text takes.
const screenMessagePositionTopCenter = 2

// FrameExShowScreenMessage shows text at the top center of the screen for
// durationMs milliseconds, as a custom text (type 1, no system message id
// -1), at the normal size, with no upper effect and no fading.
func FrameExShowScreenMessage(text string, durationMs int32) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExShowScreenMessage)
	w.WriteInt32(1)  // custom text
	w.WriteInt32(-1) // no system message
	w.WriteInt32(screenMessagePositionTopCenter)
	w.WriteInt32(0) // not hidden
	w.WriteInt32(0) // normal size
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0) // no upper effect
	w.WriteInt32(durationMs)
	w.WriteInt32(0) // no fading
	w.WriteString(text)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
