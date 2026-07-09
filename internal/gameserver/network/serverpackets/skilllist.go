package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeSkillList is the wire opcode for SkillList, the character's known
// skills sent on world entry and after any change to that set.
const OpcodeSkillList = 0x58

// SkillListEntry is one known skill as the client's skill window shows it.
type SkillListEntry struct {
	ID      int32
	Level   int32
	Passive bool
	// Disabled marks a skill greyed out in the client (worn formal wear, or
	// a clan skill while the clan's reputation is negative).
	Disabled bool
}

// FrameSkillList builds the SkillList packet for skills as an owned frame. A
// character with no skill data loaded yet (the skill system isn't modeled)
// encodes as an empty list, which is a valid, client-accepted skill window.
func FrameSkillList(skills []SkillListEntry) wire.Frame {
	w := newFrameWriter(OpcodeSkillList)
	writeSkillList(w, skills)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

func writeSkillList(w *wire.Writer, skills []SkillListEntry) {
	w.WriteInt32(int32(len(skills)))
	for _, s := range skills {
		w.WriteInt32(boolInt32(s.Passive))
		w.WriteInt32(s.Level)
		w.WriteInt32(s.ID)
		w.WriteUint8(byte(boolInt32(s.Disabled)))
	}
}
