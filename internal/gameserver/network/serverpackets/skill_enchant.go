package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// EnchantSkillEntry is one row in the enchantable skill list.
type EnchantSkillEntry struct {
	ID     int32
	Level  int32
	SPCost int32
	XPCost int64
}

// EnchantSkillRequirement is one material requirement for a skill enchant
// attempt.
type EnchantSkillRequirement struct {
	Type    int32
	ItemID  int32
	Count   int32
	Unknown int32
}

// EnchantSkillInfo contains the cost, chance, and materials for one skill
// enchant level.
type EnchantSkillInfo struct {
	ID           int32
	Level        int32
	SPCost       int32
	XPCost       int64
	Rate         int32
	Requirements []EnchantSkillRequirement
}

// FrameExEnchantSkillList builds the skill-enchant list packet.
func FrameExEnchantSkillList(skills []EnchantSkillEntry) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExEnchantSkillList)
	w.WriteInt32(int32(len(skills)))
	for _, skill := range skills {
		w.WriteInt32(skill.ID)
		w.WriteInt32(skill.Level)
		w.WriteInt32(skill.SPCost)
		w.WriteInt64(skill.XPCost)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameExEnchantSkillInfo builds the skill-enchant detail packet.
func FrameExEnchantSkillInfo(info EnchantSkillInfo) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExEnchantSkillInfo)
	w.WriteInt32(info.ID)
	w.WriteInt32(info.Level)
	w.WriteInt32(info.SPCost)
	w.WriteInt64(info.XPCost)
	w.WriteInt32(info.Rate)
	w.WriteInt32(int32(len(info.Requirements)))
	for _, req := range info.Requirements {
		w.WriteInt32(req.Type)
		w.WriteInt32(req.ItemID)
		w.WriteInt32(req.Count)
		w.WriteInt32(req.Unknown)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
