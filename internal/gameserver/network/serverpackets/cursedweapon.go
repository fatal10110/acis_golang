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
