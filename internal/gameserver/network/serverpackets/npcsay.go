package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeNpcSay is the wire opcode for NpcSay.
const OpcodeNpcSay = 0x02

const SayTypeAll int32 = 0

// FrameNpcSay builds a chat line spoken by a live NPC or summon.
func FrameNpcSay(objectID int32, npcID int, sayType int32, text string) wire.Frame {
	w := newFrameWriter(OpcodeNpcSay)
	w.WriteInt32(objectID)
	w.WriteInt32(sayType)
	w.WriteInt32(int32(1_000_000 + npcID))
	w.WriteString(text)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
