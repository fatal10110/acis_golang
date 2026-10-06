package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeQuestList is the wire opcode for QuestList.
const OpcodeQuestList = 0x80

// QuestListEntry is one quest row shown in the client's quest window.
type QuestListEntry struct {
	QuestID int32
	Flags   int32
}

// FrameQuestList builds the quest list packet.
func FrameQuestList(quests []QuestListEntry) wire.Frame {
	count, err := wire.Uint16Count(len(quests))
	if err != nil {
		return wire.InvalidFrame(err)
	}
	w := newFrameWriter(OpcodeQuestList)
	w.WriteUint16(count)
	for _, q := range quests {
		w.WriteInt32(q.QuestID)
		w.WriteInt32(q.Flags)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// OpcodeExShowQuestMark is the extended sub-opcode of ExShowQuestMark.
const OpcodeExShowQuestMark uint16 = 0x001a

// FrameExShowQuestMark builds the packet that marks questID in the client's
// quest window as moved to a new step.
func FrameExShowQuestMark(questID int32) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExShowQuestMark)
	w.WriteInt32(questID)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
