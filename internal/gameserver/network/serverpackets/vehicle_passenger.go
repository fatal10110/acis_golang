package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Boat passenger packet opcodes.
const (
	// OpcodeGetOnVehicle shows a player aboard a boat, standing at a point
	// of its deck.
	OpcodeGetOnVehicle = 0x5c
	// OpcodeGetOffVehicle shows a player stepping off a boat.
	OpcodeGetOffVehicle = 0x5d
	// OpcodeMoveToLocationInVehicle shows a player walking across a boat's
	// deck.
	OpcodeMoveToLocationInVehicle = 0x71
	// OpcodeStopMoveInVehicle stops a player's walk on a boat's deck.
	OpcodeStopMoveInVehicle = 0x72
	// OpcodeValidateLocationInVehicle corrects a player's position on a
	// boat's deck. Nothing sends it: a passenger's drift is corrected with
	// GetOnVehicle instead.
	OpcodeValidateLocationInVehicle = 0x73
)

// Boat passenger system messages.
const (
	SystemMessageEnterPeacefulZone    = 116
	SystemMessageExitPeacefulZone     = 117
	SystemMessageNotCorrectBoatTicket = 402
	SystemMessageReleasePetOnBoat     = 1523
)

// FrameGetOnVehicle builds the packet showing player objectID aboard boat
// boatID at deck point at.
func FrameGetOnVehicle(objectID, boatID int32, at location.Location) wire.Frame {
	return framePassenger(OpcodeGetOnVehicle, objectID, boatID, at)
}

// FrameGetOffVehicle builds the packet showing player objectID stepping off
// boat boatID toward at.
func FrameGetOffVehicle(objectID, boatID int32, at location.Location) wire.Frame {
	return framePassenger(OpcodeGetOffVehicle, objectID, boatID, at)
}

func framePassenger(opcode byte, objectID, boatID int32, at location.Location) wire.Frame {
	w := newFrameWriter(opcode)
	w.WriteInt32(objectID)
	w.WriteInt32(boatID)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameMoveToLocationInVehicle builds the packet showing player objectID
// walking from deck point origin to deck point target of boat boatID.
func FrameMoveToLocationInVehicle(objectID, boatID int32, target, origin location.Location) wire.Frame {
	w := newFrameWriter(OpcodeMoveToLocationInVehicle)
	w.WriteInt32(objectID)
	w.WriteInt32(boatID)
	w.WriteInt32(int32(target.X))
	w.WriteInt32(int32(target.Y))
	w.WriteInt32(int32(target.Z))
	w.WriteInt32(int32(origin.X))
	w.WriteInt32(int32(origin.Y))
	w.WriteInt32(int32(origin.Z))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameStopMoveInVehicle builds the packet stopping player objectID at deck
// point at of boat boatID, facing heading.
func FrameStopMoveInVehicle(objectID, boatID int32, at location.Location, heading int) wire.Frame {
	return frameDeckPosition(OpcodeStopMoveInVehicle, objectID, boatID, at, heading)
}

// FrameValidateLocationInVehicle builds the packet setting player objectID
// at deck point at of boat boatID, facing heading.
func FrameValidateLocationInVehicle(objectID, boatID int32, at location.Location, heading int) wire.Frame {
	return frameDeckPosition(OpcodeValidateLocationInVehicle, objectID, boatID, at, heading)
}

func frameDeckPosition(opcode byte, objectID, boatID int32, at location.Location, heading int) wire.Frame {
	w := newFrameWriter(opcode)
	w.WriteInt32(objectID)
	w.WriteInt32(boatID)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	w.WriteInt32(int32(heading))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
