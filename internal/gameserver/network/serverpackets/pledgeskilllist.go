package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeExPledgeSkillListAdd is the extended sub-opcode of the clan skill
// list's single-skill addition.
const OpcodeExPledgeSkillListAdd uint16 = 0x003a

// FramePledgeSkillList builds a clan skill list packet.
func FramePledgeSkillList(skills []SkillListEntry) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExPledgeSkillList)
	w.WriteInt32(int32(len(skills)))
	for _, skill := range skills {
		w.WriteInt32(skill.ID)
		w.WriteInt32(skill.Level)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FramePledgeSkillListAdd builds the clan skill list's addition of one
// skill at its new level.
func FramePledgeSkillListAdd(id, level int32) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExPledgeSkillListAdd)
	w.WriteInt32(id)
	w.WriteInt32(level)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
