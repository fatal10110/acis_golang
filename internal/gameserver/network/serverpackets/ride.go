package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

const OpcodeRide = 0x86

// FrameRide builds the mount transition packet for a player and NPC template.
func FrameRide(objectID, npcID int32) wire.Frame {
	return frameRide(objectID, 1, npcID)
}

// FrameDismount builds the dismount transition packet for a player.
func FrameDismount(objectID int32) wire.Frame {
	return frameRide(objectID, 0, 0)
}

// frameRide writes the Ride packet: the action (1 mount, 0 dismount), the
// ride type of npcID and the ride class, npcID + 1000000.
func frameRide(objectID, action, npcID int32) wire.Frame {
	w := newFrameWriter(OpcodeRide)
	w.WriteInt32(objectID)
	w.WriteInt32(action)
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
