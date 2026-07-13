package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// OpcodeMoveToLocation is the wire opcode for MoveToLocation, sent when a
// visible creature starts moving toward a world coordinate.
const OpcodeMoveToLocation = 0x01

// FrameMoveToLocation builds the MoveToLocation packet as an owned frame.
func FrameMoveToLocation(objectID int32, destination, origin location.Location) wire.Frame {
	w := newFrameWriter(OpcodeMoveToLocation)
	w.WriteInt32(objectID)
	w.WriteInt32(int32(destination.X))
	w.WriteInt32(int32(destination.Y))
	w.WriteInt32(int32(destination.Z))
	w.WriteInt32(int32(origin.X))
	w.WriteInt32(int32(origin.Y))
	w.WriteInt32(int32(origin.Z))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
