package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

const (
	// OpcodeAcquireSkillList is the wire opcode for AcquireSkillList.
	OpcodeAcquireSkillList = 0x8a
	// OpcodeAcquireSkillInfo is the wire opcode for AcquireSkillInfo.
	OpcodeAcquireSkillInfo = 0x8b
	// OpcodeAcquireSkillDone is the wire opcode for AcquireSkillDone.
	OpcodeAcquireSkillDone = 0x8e
)

// AcquireSkillType is the trainer list mode sent in AcquireSkillList.
type AcquireSkillType int32

const (
	// AcquireSkillTypeUsual is the ordinary class trainer skill list.
	AcquireSkillTypeUsual AcquireSkillType = 0
	// AcquireSkillTypeFishing is the common/fishing skill list.
	AcquireSkillTypeFishing AcquireSkillType = 1
	// AcquireSkillTypeClan is the clan skill list.
	AcquireSkillTypeClan AcquireSkillType = 2
)

// AcquireSkillListEntry is one row in an AcquireSkillList packet.
type AcquireSkillListEntry struct {
	ID      int32
	Level   int32
	Cost    int32
	Unknown int32
}

// SkillRequirement is one AcquireSkillInfo material requirement.
type SkillRequirement struct {
	Type    int32
	ItemID  int32
	Count   int32
	Unknown int32
}

// FrameAcquireSkillList builds an AcquireSkillList packet.
func FrameAcquireSkillList(skillType AcquireSkillType, skills []AcquireSkillListEntry) wire.Frame {
	w := newFrameWriter(OpcodeAcquireSkillList)
	w.WriteInt32(int32(skillType))
	w.WriteInt32(int32(len(skills)))
	for _, skill := range skills {
		w.WriteInt32(skill.ID)
		w.WriteInt32(skill.Level)
		w.WriteInt32(skill.Level)
		w.WriteInt32(skill.Cost)
		w.WriteInt32(skill.Unknown)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameAcquireSkillInfo builds an AcquireSkillInfo packet.
func FrameAcquireSkillInfo(id, level, spCost, mode int32, reqs []SkillRequirement) wire.Frame {
	w := newFrameWriter(OpcodeAcquireSkillInfo)
	w.WriteInt32(id)
	w.WriteInt32(level)
	w.WriteInt32(spCost)
	w.WriteInt32(mode)
	w.WriteInt32(int32(len(reqs)))
	for _, req := range reqs {
		w.WriteInt32(req.Type)
		w.WriteInt32(req.ItemID)
		w.WriteInt32(req.Count)
		w.WriteInt32(req.Unknown)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameAcquireSkillDone builds an AcquireSkillDone packet.
func FrameAcquireSkillDone() wire.Frame {
	w := newFrameWriter(OpcodeAcquireSkillDone)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
