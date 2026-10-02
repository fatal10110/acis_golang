package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeCameraMode is the wire opcode for CameraMode, which switches the
// client's camera.
const OpcodeCameraMode = 0xf1

// CameraModeThirdPerson and CameraModeFirstPerson are the camera modes
// CameraMode carries: the ordinary camera, and the free camera a game
// master's //camera turns on.
const (
	CameraModeThirdPerson int32 = 0
	CameraModeFirstPerson int32 = 1
)

// FrameCameraMode builds the CameraMode packet as an owned frame.
func FrameCameraMode(mode int32) wire.Frame {
	w := newFrameWriter(OpcodeCameraMode)
	w.WriteInt32(mode)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
