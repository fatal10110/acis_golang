package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Boat packet opcodes.
const (
	// OpcodeVehicleInfo shows a boat where it stands, facing where it faces.
	OpcodeVehicleInfo = 0x59
	// OpcodeVehicleDeparture sets a boat moving toward a point at a given
	// move and rotation speed.
	OpcodeVehicleDeparture = 0x5a
	// OpcodeOnVehicleCheckLocation corrects a passenger's view of the boat
	// it rides on.
	OpcodeOnVehicleCheckLocation = 0x5b
	// OpcodeVehicleStarted tells whether a boat runs its route.
	OpcodeVehicleStarted = 0xba
)

// boatSayStringID is the client sysstring every boat schedule line is shown
// under.
const boatSayStringID = 801

// boatChatChannel is the boat chat channel's number on the wire.
const boatChatChannel = 11

// FrameVehicleInfo builds the packet showing boat objectID at at, facing
// heading.
func FrameVehicleInfo(objectID int32, at location.Location, heading int) wire.Frame {
	return frameVehiclePosition(OpcodeVehicleInfo, objectID, at, heading)
}

// FrameOnVehicleCheckLocation builds the passenger's correction of boat
// objectID to at, facing heading.
func FrameOnVehicleCheckLocation(objectID int32, at location.Location, heading int) wire.Frame {
	return frameVehiclePosition(OpcodeOnVehicleCheckLocation, objectID, at, heading)
}

func frameVehiclePosition(opcode byte, objectID int32, at location.Location, heading int) wire.Frame {
	w := newFrameWriter(opcode)
	w.WriteInt32(objectID)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	w.WriteInt32(int32(heading))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameVehicleDeparture builds the packet setting boat objectID moving
// toward destination at moveSpeed, turning at rotationSpeed.
func FrameVehicleDeparture(objectID int32, moveSpeed, rotationSpeed int, destination location.Location) wire.Frame {
	w := newFrameWriter(OpcodeVehicleDeparture)
	w.WriteInt32(objectID)
	w.WriteInt32(int32(moveSpeed))
	w.WriteInt32(int32(rotationSpeed))
	w.WriteInt32(int32(destination.X))
	w.WriteInt32(int32(destination.Y))
	w.WriteInt32(int32(destination.Z))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameVehicleStarted builds the packet telling that boat objectID set off
// on its route (moving) or stopped at the end of it.
func FrameVehicleStarted(objectID int32, moving bool) wire.Frame {
	w := newFrameWriter(OpcodeVehicleStarted)
	w.WriteInt32(objectID)
	w.WriteInt32(boolInt32(moving))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameBoatSay builds a boat schedule line: system message messageID shown
// on the boat chat channel, said by no one.
func FrameBoatSay(messageID int) wire.Frame {
	w := newFrameWriter(OpcodeCreatureSay)
	w.WriteInt32(0)
	w.WriteInt32(boatChatChannel)
	w.WriteInt32(boatSayStringID)
	w.WriteInt32(int32(messageID))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
