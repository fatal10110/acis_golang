package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

const (
	// OpcodeObserverStart puts the client in observer mode at a
	// viewpoint.
	OpcodeObserverStart = 0xdf
	// OpcodeObserverEnd takes the client out of observer mode, back to
	// the position it left.
	OpcodeObserverEnd = 0xe0
)

// System messages of the broadcasting towers' viewpoints.
const (
	// SystemMessageOnlyViewSiege: a castle viewpoint is only open during
	// that castle's siege.
	SystemMessageOnlyViewSiege = 780
	// SystemMessageNoObserveWithPet: a castle viewpoint refuses a player
	// with a summon out.
	SystemMessageNoObserveWithPet = 782
	// SystemMessageCannotObserveInCombat: no viewpoint takes a player in
	// combat.
	SystemMessageCannotObserveInCombat = 1599
)

// FrameObserverStart builds the ObserverStart packet: the viewpoint's
// position, then its camera yaw and pitch.
func FrameObserverStart(at location.Location, yaw, pitch int) wire.Frame {
	w := newFrameWriter(OpcodeObserverStart)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	w.WriteInt32(int32(yaw))
	w.WriteInt32(int32(pitch))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameObserverEnd builds the ObserverEnd packet: the position the
// observer returns to.
func FrameObserverEnd(to location.Location) wire.Frame {
	w := newFrameWriter(OpcodeObserverEnd)
	w.WriteInt32(int32(to.X))
	w.WriteInt32(int32(to.Y))
	w.WriteInt32(int32(to.Z))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
