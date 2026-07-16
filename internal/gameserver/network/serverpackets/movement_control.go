package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

const (
	// OpcodeStopMove is the wire opcode for StopMove.
	OpcodeStopMove byte = 0x47
	// OpcodeValidateLocation is the wire opcode for ValidateLocation.
	OpcodeValidateLocation byte = 0x61
	// OpcodeStartRotation is the wire opcode for StartRotation.
	OpcodeStartRotation byte = 0x62
	// OpcodeStopRotation is the wire opcode for StopRotation.
	OpcodeStopRotation byte = 0x63
)

// FrameStopMove builds a stopped-movement correction packet.
func FrameStopMove(objectID int32, at location.Location, heading int) wire.Frame {
	w := newFrameWriter(OpcodeStopMove)
	w.WriteInt32(objectID)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	w.WriteInt32(int32(heading))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameValidateLocation builds a position correction packet.
func FrameValidateLocation(objectID int32, at location.Location, heading int) wire.Frame {
	w := newFrameWriter(OpcodeValidateLocation)
	w.WriteInt32(objectID)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	w.WriteInt32(int32(heading))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameStartRotation builds a rotation-start broadcast packet.
func FrameStartRotation(objectID int32, degree, side, speed int) wire.Frame {
	w := newFrameWriter(OpcodeStartRotation)
	w.WriteInt32(objectID)
	w.WriteInt32(int32(degree))
	w.WriteInt32(int32(side))
	w.WriteInt32(int32(speed))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameStopRotation builds a rotation-stop broadcast packet.
func FrameStopRotation(objectID int32, degree, speed int) wire.Frame {
	w := newFrameWriter(OpcodeStopRotation)
	w.WriteInt32(objectID)
	w.WriteInt32(int32(degree))
	w.WriteInt32(int32(speed))
	w.WriteUint8(uint8(degree))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
