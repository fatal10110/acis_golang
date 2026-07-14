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
	w := newFrameWriter(OpcodeQuestList)
	w.WriteUint16(uint16(len(quests)))
	for _, q := range quests {
		w.WriteInt32(q.QuestID)
		w.WriteInt32(q.Flags)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
