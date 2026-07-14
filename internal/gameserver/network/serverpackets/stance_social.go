package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

const (
	// OpcodeAutoAttackStart is the wire opcode for AutoAttackStart.
	OpcodeAutoAttackStart byte = 0x2b
	// OpcodeSocialAction is the wire opcode for SocialAction.
	OpcodeSocialAction byte = 0x2d
	// OpcodeChangeMoveType is the wire opcode for ChangeMoveType.
	OpcodeChangeMoveType byte = 0x2e
	// OpcodeChangeWaitType is the wire opcode for ChangeWaitType.
	OpcodeChangeWaitType byte = 0x2f
)

// WaitType is the animation mode used by ChangeWaitType.
type WaitType int32

const (
	WaitSitting  WaitType = 0
	WaitStanding WaitType = 1
)

// FrameAutoAttackStart builds the attack-stance start broadcast packet.
func FrameAutoAttackStart(objectID int32) wire.Frame {
	w := newFrameWriter(OpcodeAutoAttackStart)
	w.WriteInt32(objectID)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameSocialAction builds a social animation broadcast packet.
func FrameSocialAction(objectID int32, actionID int32) wire.Frame {
	w := newFrameWriter(OpcodeSocialAction)
	w.WriteInt32(objectID)
	w.WriteInt32(actionID)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameChangeMoveType builds a walk/run stance broadcast packet.
func FrameChangeMoveType(objectID int32, running, swimming bool) wire.Frame {
	w := newFrameWriter(OpcodeChangeMoveType)
	w.WriteInt32(objectID)
	w.WriteInt32(boolInt32(running))
	w.WriteInt32(boolInt32(swimming))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameChangeWaitType builds a sit/stand stance broadcast packet.
func FrameChangeWaitType(objectID int32, waitType WaitType, at location.Location) wire.Frame {
	w := newFrameWriter(OpcodeChangeWaitType)
	w.WriteInt32(objectID)
	w.WriteInt32(int32(waitType))
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
