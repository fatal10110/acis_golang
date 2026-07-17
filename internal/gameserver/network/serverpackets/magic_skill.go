package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

const (
	// OpcodeMagicSkillUse is the wire opcode for MagicSkillUse.
	OpcodeMagicSkillUse byte = 0x48
	// OpcodeSetupGauge is the wire opcode for SetupGauge.
	OpcodeSetupGauge byte = 0x6d
	// OpcodeMagicSkillLaunched is the wire opcode for MagicSkillLaunched.
	OpcodeMagicSkillLaunched byte = 0x76
	// OpcodeMagicSkillCanceled is the wire opcode for MagicSkillCanceled.
	OpcodeMagicSkillCanceled byte = 0x49
)

// GaugeColor is the client gauge color ordinal.
type GaugeColor int32

const (
	GaugeBlue GaugeColor = iota
	GaugeRed
	GaugeCyan
	GaugeGreen
)

// SkillCastObject is one caster or target endpoint in a skill animation
// packet.
type SkillCastObject struct {
	ObjectID int32
	Location location.Location
}

// FrameMagicSkillUse builds the cast-start animation packet.
func FrameMagicSkillUse(caster, target SkillCastObject, skillID, level int32, hitTime, reuseDelay int, success bool) wire.Frame {
	w := newFrameWriter(OpcodeMagicSkillUse)
	w.WriteInt32(caster.ObjectID)
	w.WriteInt32(target.ObjectID)
	w.WriteInt32(skillID)
	w.WriteInt32(level)
	w.WriteInt32(int32(hitTime))
	w.WriteInt32(int32(reuseDelay))
	w.WriteInt32(int32(caster.Location.X))
	w.WriteInt32(int32(caster.Location.Y))
	w.WriteInt32(int32(caster.Location.Z))
	if success {
		w.WriteInt32(1)
		w.WriteUint16(0)
	} else {
		w.WriteInt32(0)
	}
	w.WriteInt32(int32(target.Location.X))
	w.WriteInt32(int32(target.Location.Y))
	w.WriteInt32(int32(target.Location.Z))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameMagicSkillLaunched builds the cast-launch target packet.
func FrameMagicSkillLaunched(objectID, skillID, level int32, targetIDs []int32) wire.Frame {
	w := newFrameWriter(OpcodeMagicSkillLaunched)
	w.WriteInt32(objectID)
	w.WriteInt32(skillID)
	w.WriteInt32(level)
	if len(targetIDs) == 0 {
		w.WriteInt32(0)
		w.WriteInt32(0)
	} else {
		w.WriteInt32(int32(len(targetIDs)))
		for _, targetID := range targetIDs {
			w.WriteInt32(targetID)
		}
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameMagicSkillCanceled builds the cast-cancel animation packet.
func FrameMagicSkillCanceled(objectID int32) wire.Frame {
	w := newFrameWriter(OpcodeMagicSkillCanceled)
	w.WriteInt32(objectID)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameSetupGauge builds a cast/progress gauge packet.
func FrameSetupGauge(color GaugeColor, currentTime, maxTime int) wire.Frame {
	w := newFrameWriter(OpcodeSetupGauge)
	w.WriteInt32(int32(color))
	w.WriteInt32(int32(currentTime))
	w.WriteInt32(int32(maxTime))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
