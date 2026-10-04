package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

type CursedWeaponLocation struct {
	ItemID   int32
	Active   bool
	Location location.Location
}

// FrameExCursedWeaponList builds the cursed weapon item-id list.
func FrameExCursedWeaponList(ids []int32) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExCursedWeaponList)
	w.WriteInt32(int32(len(ids)))
	for _, id := range ids {
		w.WriteInt32(id)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameExCursedWeaponLocation builds the active cursed weapon location list.
func FrameExCursedWeaponLocation(entries []CursedWeaponLocation) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExCursedWeaponLocation)
	if len(entries) == 0 {
		w.WriteInt32(0)
		w.WriteInt32(0)
		return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
	}

	w.WriteInt32(int32(len(entries)))
	for _, entry := range entries {
		w.WriteInt32(entry.ItemID)
		w.WriteInt32(wire.BoolInt32(entry.Active))
		w.WriteInt32(int32(entry.Location.X))
		w.WriteInt32(int32(entry.Location.Y))
		w.WriteInt32(int32(entry.Location.Z))
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// OpcodeEarthquake is the wire opcode for Earthquake, which shakes the
// screen of every client near a point.
const OpcodeEarthquake = 0xc4

// FrameEarthquake builds an Earthquake at (x, y, z) of the given intensity
// lasting duration seconds; npc marks one an NPC causes.
func FrameEarthquake(x, y, z int, intensity, duration int32, npc bool) wire.Frame {
	w := newFrameWriter(OpcodeEarthquake)
	w.WriteInt32(int32(x))
	w.WriteInt32(int32(y))
	w.WriteInt32(int32(z))
	w.WriteInt32(intensity)
	w.WriteInt32(duration)
	w.WriteInt32(wire.BoolInt32(npc))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// Cursed weapon system message ids.
const (
	SystemMessageS2HoursOfUsageTimeLeftForS1   = 1813 // item-name then number parameter
	SystemMessageS2MinutesOfUsageTimeLeftForS1 = 1814 // item-name then number parameter
	SystemMessageS2WasDroppedInTheS1Region     = 1815 // zone-name then item-name parameter
	SystemMessageOwnerOfS2AppearedInS1Region   = 1816 // zone-name then item-name parameter
	SystemMessageS2OwnerLoggedIntoS1Region     = 1817 // zone-name then item-name parameter
	SystemMessageS1HasDisappeared              = 1818 // item-name parameter
)
