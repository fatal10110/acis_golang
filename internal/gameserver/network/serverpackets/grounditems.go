package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

const (
	// OpcodeSpawnItem is the wire opcode for a ground item entering sight.
	OpcodeSpawnItem = 0x0b
	// OpcodeDropItem is the wire opcode for the animated item drop packet.
	OpcodeDropItem = 0x0c
	// OpcodeGetItem is the wire opcode for the animated item pickup packet.
	OpcodeGetItem = 0x0d
)

type groundItemPacketObject interface {
	ObjectID() int32
	ItemID() int32
	Count() int
	Stackable() bool
	Position() (int, int, int)
}

// FrameSpawnItem builds the static ground-item spawn packet.
func FrameSpawnItem(ground groundItemPacketObject) wire.Frame {
	x, y, z := ground.Position()
	w := newFrameWriter(OpcodeSpawnItem)
	w.WriteInt32(ground.ObjectID())
	w.WriteInt32(ground.ItemID())
	w.WriteInt32(int32(x))
	w.WriteInt32(int32(y))
	w.WriteInt32(int32(z))
	w.WriteInt32(boolInt32(ground.Stackable()))
	w.WriteInt32(int32(ground.Count()))
	w.WriteInt32(0)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameDropItem builds the animated item drop packet.
func FrameDropItem(ground groundItemPacketObject, dropperID int32) wire.Frame {
	x, y, z := ground.Position()
	w := newFrameWriter(OpcodeDropItem)
	w.WriteInt32(dropperID)
	w.WriteInt32(ground.ObjectID())
	w.WriteInt32(ground.ItemID())
	w.WriteInt32(int32(x))
	w.WriteInt32(int32(y))
	w.WriteInt32(int32(z))
	w.WriteInt32(boolInt32(ground.Stackable()))
	w.WriteInt32(int32(ground.Count()))
	w.WriteInt32(1)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameGetItem builds the animated item pickup packet.
func FrameGetItem(ground groundItemPacketObject, pickerID int32) wire.Frame {
	x, y, z := ground.Position()
	w := newFrameWriter(OpcodeGetItem)
	w.WriteInt32(pickerID)
	w.WriteInt32(ground.ObjectID())
	w.WriteInt32(int32(x))
	w.WriteInt32(int32(y))
	w.WriteInt32(int32(z))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
