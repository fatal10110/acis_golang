package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
)

// OpcodeSpecialCamera is the wire opcode for SpecialCamera, which plays a
// cutscene camera shot around an object.
const OpcodeSpecialCamera = 0xc7

// FrameSpecialCamera builds the SpecialCamera packet for shot, centred on
// objectID, as an owned frame.
func FrameSpecialCamera(objectID int32, shot event.CameraShot) wire.Frame {
	w := newFrameWriter(OpcodeSpecialCamera)
	w.WriteInt32(objectID)
	w.WriteInt32(shot.Distance)
	w.WriteInt32(shot.Yaw)
	w.WriteInt32(shot.Pitch)
	w.WriteInt32(shot.Time)
	w.WriteInt32(shot.Duration)
	w.WriteInt32(shot.Turn)
	w.WriteInt32(shot.Rise)
	w.WriteInt32(shot.Widescreen)
	w.WriteInt32(shot.Unknown)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// OpcodeExPlayScene is the extended opcode of ExPlayScene, which plays the
// client's built-in scene. The packet has no body.
const OpcodeExPlayScene uint16 = 0x005b

// FrameExPlayScene builds the ExPlayScene packet as an owned frame.
func FrameExPlayScene() wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExPlayScene)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
