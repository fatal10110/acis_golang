package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

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
