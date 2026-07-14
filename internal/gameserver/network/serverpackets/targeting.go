package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

const (
	// OpcodeStatusUpdate is the wire opcode for StatusUpdate.
	OpcodeStatusUpdate byte = 0x0e
	// OpcodeTargetSelected is the wire opcode broadcast when a player selects a target.
	OpcodeTargetSelected byte = 0x29
	// OpcodeTargetUnselected is the wire opcode broadcast when a player clears target.
	OpcodeTargetUnselected byte = 0x2a
	// OpcodeMyTargetSelected is the wire opcode sent to the selecting player.
	OpcodeMyTargetSelected byte = 0xa6
)

// StatusType is the numeric id used by StatusUpdate attributes.
type StatusType int32

const (
	StatusCurrentHP StatusType = 9
	StatusMaxHP     StatusType = 10
)

// StatusAttribute is one type/value pair in a StatusUpdate packet.
type StatusAttribute struct {
	Type  StatusType
	Value int
}

// FrameMyTargetSelected builds the selecting client's target confirmation.
func FrameMyTargetSelected(objectID int32, color int) wire.Frame {
	w := newFrameWriter(OpcodeMyTargetSelected)
	w.WriteInt32(objectID)
	w.WriteUint16(uint16(color))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameTargetSelected builds the observer broadcast for a selected target.
func FrameTargetSelected(objectID, targetID int32, at location.Location) wire.Frame {
	w := newFrameWriter(OpcodeTargetSelected)
	w.WriteInt32(objectID)
	w.WriteInt32(targetID)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameTargetUnselected builds the observer broadcast for a cleared target.
func FrameTargetUnselected(objectID int32, at location.Location) wire.Frame {
	w := newFrameWriter(OpcodeTargetUnselected)
	w.WriteInt32(objectID)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameStatusUpdate builds a StatusUpdate packet for one object.
func FrameStatusUpdate(objectID int32, attrs []StatusAttribute) wire.Frame {
	w := newFrameWriter(OpcodeStatusUpdate)
	w.WriteInt32(objectID)
	w.WriteInt32(int32(len(attrs)))
	for _, attr := range attrs {
		w.WriteInt32(int32(attr.Type))
		w.WriteInt32(int32(attr.Value))
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
