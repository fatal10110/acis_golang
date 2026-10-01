package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeCreatureSay is a chat line shown in the chat window and, when it
// names a speaker in view, over the speaker's head.
const OpcodeCreatureSay = 0x4a

// FrameCreatureSay builds a chat line said by objectID (0 for none) on chat
// channel sayType, shown after name.
func FrameCreatureSay(objectID, sayType int32, name, text string) wire.Frame {
	w := newFrameWriter(OpcodeCreatureSay)
	w.WriteInt32(objectID)
	w.WriteInt32(sayType)
	w.WriteString(name)
	w.WriteString(text)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
