package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// OpcodeMoveToPawn is the wire opcode for MoveToPawn, sent when a visible
// creature moves toward a target object.
const OpcodeMoveToPawn = 0x60

// FrameMoveToPawn builds the MoveToPawn packet as an owned frame.
func FrameMoveToPawn(objectID, targetID int32, distance int, origin location.Location) wire.Frame {
	w := newFrameWriter(OpcodeMoveToPawn)
	w.WriteInt32(objectID)
	w.WriteInt32(targetID)
	w.WriteInt32(int32(distance))
	w.WriteInt32(int32(origin.X))
	w.WriteInt32(int32(origin.Y))
	w.WriteInt32(int32(origin.Z))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
