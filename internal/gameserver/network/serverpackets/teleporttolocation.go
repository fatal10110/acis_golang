package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// OpcodeTeleportToLocation is the wire opcode for TeleportToLocation, sent
// when an object's position changes discontinuously (as opposed to a
// walked/run movement).
const OpcodeTeleportToLocation = 0x28

// FrameTeleportToLocation builds the TeleportToLocation packet as an owned
// frame. fastTeleport selects the client's transition: false shows a black
// screen, true is a fast in-place position correction.
func FrameTeleportToLocation(objectID int32, to location.Location, fastTeleport bool) wire.Frame {
	w := newFrameWriter(OpcodeTeleportToLocation)
	w.WriteInt32(objectID)
	w.WriteInt32(int32(to.X))
	w.WriteInt32(int32(to.Y))
	w.WriteInt32(int32(to.Z))
	if fastTeleport {
		w.WriteInt32(1)
	} else {
		w.WriteInt32(0)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
