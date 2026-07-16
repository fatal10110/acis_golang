package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

const (
	// OpcodeDoorInfo is the wire opcode for DoorInfo, the initial door state.
	OpcodeDoorInfo = 0x4c
	// OpcodeDoorStatusUpdate is the wire opcode for a door state update.
	OpcodeDoorStatusUpdate = 0x4d
	// OpcodeStaticObjectInfo is the wire opcode for StaticObjectInfo.
	OpcodeStaticObjectInfo = 0x99
	// OpcodeChairSit is the wire opcode for ChairSit.
	OpcodeChairSit = 0xe1
)

type doorPacketObject interface {
	ObjectID() int32
	DoorID() int
	Opened() bool
	MaxHP() int
	HP() int
	Damage() int
}

type staticPacketObject interface {
	ObjectID() int32
	StaticObjectID() int
}

// FrameDoorInfo builds the initial door info packet.
func FrameDoorInfo(d doorPacketObject, showHP bool) wire.Frame {
	w := newFrameWriter(OpcodeDoorInfo)
	w.WriteInt32(d.ObjectID())
	w.WriteInt32(int32(d.DoorID()))
	w.WriteInt32(boolInt32(showHP))
	w.WriteInt32(1)
	w.WriteInt32(boolInt32(!d.Opened()))
	w.WriteInt32(int32(d.MaxHP()))
	w.WriteInt32(int32(d.HP()))
	w.WriteInt32(0)
	w.WriteInt32(0)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameDoorStatusUpdate builds a door state update packet.
func FrameDoorStatusUpdate(d doorPacketObject, showHP bool) wire.Frame {
	w := newFrameWriter(OpcodeDoorStatusUpdate)
	w.WriteInt32(d.ObjectID())
	w.WriteInt32(boolInt32(!d.Opened()))
	w.WriteInt32(int32(d.Damage()))
	w.WriteInt32(boolInt32(showHP))
	w.WriteInt32(int32(d.DoorID()))
	w.WriteInt32(int32(d.MaxHP()))
	w.WriteInt32(int32(d.HP()))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameStaticObjectInfo builds the static object info packet.
func FrameStaticObjectInfo(obj staticPacketObject) wire.Frame {
	w := newFrameWriter(OpcodeStaticObjectInfo)
	w.WriteInt32(int32(obj.StaticObjectID()))
	w.WriteInt32(obj.ObjectID())
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameChairSit builds the static-object chair sit packet.
func FrameChairSit(playerID int32, staticID int) wire.Frame {
	w := newFrameWriter(OpcodeChairSit)
	w.WriteInt32(playerID)
	w.WriteInt32(int32(staticID))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
