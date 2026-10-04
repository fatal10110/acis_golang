package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeTitleUpdate is the wire opcode for TitleUpdate.
const OpcodeTitleUpdate = 0xcc

// FrameTitleUpdate builds the packet that redraws objectID's title as
// title.
func FrameTitleUpdate(objectID int32, title string) wire.Frame {
	w := newFrameWriter(OpcodeTitleUpdate)
	w.WriteInt32(objectID)
	w.WriteString(title)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
