package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

const OpcodeRide = 0x86

// FrameRide builds the mount transition packet for a player and NPC template.
func FrameRide(objectID, npcID int32) wire.Frame {
	w := newFrameWriter(OpcodeRide)
	w.WriteInt32(objectID)
	w.WriteInt32(1)
	w.WriteInt32(mountType(npcID))
	w.WriteInt32(npcID + 1_000_000)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

func mountType(npcID int32) int32 {
	if npcID == 12621 {
		return 2
	}
	return 0
}
