package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeSkillCoolTime is the wire opcode for SkillCoolTime.
const OpcodeSkillCoolTime = 0xc1

// SkillCoolTimeEntry is one pending skill reuse timer.
type SkillCoolTimeEntry struct {
	SkillID          int32
	Level            int32
	ReuseSeconds     int32
	RemainingSeconds int32
}

// FrameSkillCoolTime builds the pending skill reuse packet.
func FrameSkillCoolTime(entries []SkillCoolTimeEntry) wire.Frame {
	w := newFrameWriter(OpcodeSkillCoolTime)
	w.WriteInt32(int32(len(entries)))
	for _, e := range entries {
		w.WriteInt32(e.SkillID)
		w.WriteInt32(e.Level)
		w.WriteInt32(e.ReuseSeconds)
		w.WriteInt32(e.RemainingSeconds)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
