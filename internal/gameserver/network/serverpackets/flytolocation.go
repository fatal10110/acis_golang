package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// OpcodeFlyToLocation is the wire opcode for FlyToLocation.
const OpcodeFlyToLocation byte = 0xC5

// FrameFlyToLocation builds a forced-flight animation packet: the client
// plays the actor moving from its current position to dest, tagged with the
// trajectory kind (knockback, charge, ...). The server applies the actual
// XYZ change separately once the effect driving the flight ends.
func FrameFlyToLocation(objectID int32, dest, at location.Location, flight skill.Flight) wire.Frame {
	w := newFrameWriter(OpcodeFlyToLocation)
	w.WriteInt32(objectID)
	w.WriteInt32(int32(dest.X))
	w.WriteInt32(int32(dest.Y))
	w.WriteInt32(int32(dest.Z))
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	w.WriteInt32(int32(flight))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
