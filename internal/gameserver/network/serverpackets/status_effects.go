package serverpackets

import (
	"sort"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

const (
	// OpcodeAbnormalStatusUpdate is the wire opcode for AbnormalStatusUpdate.
	OpcodeAbnormalStatusUpdate byte = 0x7f
	// OpcodeShortBuffStatusUpdate is the wire opcode for ShortBuffStatusUpdate.
	OpcodeShortBuffStatusUpdate byte = 0xf4
)

// AbnormalStatusEffect is one effect entry in AbnormalStatusUpdate.
type AbnormalStatusEffect struct {
	SkillID        int32
	Level          int32
	DurationMillis int
	Toggle         bool
}

// FrameAbnormalStatusUpdate builds the active abnormal effect list packet.
func FrameAbnormalStatusUpdate(effects []AbnormalStatusEffect) wire.Frame {
	normal, toggles := splitAbnormalEffects(effects)
	count, err := wire.Uint16Count(len(normal) + len(toggles))
	if err != nil {
		return wire.InvalidFrame(err)
	}

	w := newFrameWriter(OpcodeAbnormalStatusUpdate)
	w.WriteUint16(count)
	for _, e := range normal {
		writeAbnormalStatusEffect(w, e, e.DurationMillis == -1)
	}
	for _, e := range toggles {
		writeAbnormalStatusEffect(w, e, true)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameShortBuffStatusUpdate builds the compact short-buff status packet.
func FrameShortBuffStatusUpdate(skillID, level, duration int32) wire.Frame {
	w := newFrameWriter(OpcodeShortBuffStatusUpdate)
	w.WriteInt32(skillID)
	w.WriteInt32(level)
	w.WriteInt32(duration)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

func splitAbnormalEffects(effects []AbnormalStatusEffect) ([]AbnormalStatusEffect, []AbnormalStatusEffect) {
	normal := make([]AbnormalStatusEffect, 0, len(effects))
	toggles := make([]AbnormalStatusEffect, 0)
	seenToggle := make(map[int32]bool)
	for _, e := range effects {
		if !e.Toggle {
			normal = append(normal, e)
			continue
		}
		if seenToggle[e.SkillID] {
			continue
		}
		seenToggle[e.SkillID] = true
		toggles = append(toggles, e)
	}
	sort.Slice(toggles, func(i, j int) bool {
		return toggles[i].SkillID < toggles[j].SkillID
	})
	return normal, toggles
}

func writeAbnormalStatusEffect(w *wire.Writer, e AbnormalStatusEffect, permanent bool) {
	w.WriteInt32(e.SkillID)
	w.WriteUint16(uint16(e.Level))
	if permanent {
		w.WriteInt32(-1)
		return
	}
	w.WriteInt32(int32(e.DurationMillis / 1000))
}

// OpcodePartySpelled is the wire opcode for PartySpelled, the effect icon
// list a party sees of one member or of a member's summon.
const OpcodePartySpelled byte = 0xee

// PartySpelled owner kinds.
const (
	PartySpelledPlayer   int32 = 0
	PartySpelledPet      int32 = 1
	PartySpelledServitor int32 = 2
)

// FramePartySpelled builds the icon list of objectID, a kind owner, as its
// party sees it. Every entry reports its whole seconds left; a permanent
// one's -1 ms reads as 0, and toggles get no special encoding.
func FramePartySpelled(kind, objectID int32, effects []AbnormalStatusEffect) wire.Frame {
	w := newFrameWriter(OpcodePartySpelled)
	w.WriteInt32(kind)
	w.WriteInt32(objectID)
	w.WriteInt32(int32(len(effects)))
	for _, e := range effects {
		w.WriteInt32(e.SkillID)
		w.WriteUint16(uint16(e.Level))
		w.WriteInt32(int32(e.DurationMillis / 1000))
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
