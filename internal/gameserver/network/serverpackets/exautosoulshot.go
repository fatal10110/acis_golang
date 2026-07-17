package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// FrameExAutoSoulShot builds the auto-shot toggle acknowledgement.
func FrameExAutoSoulShot(itemID int32, enabled bool) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExAutoSoulShot)
	w.WriteInt32(itemID)
	if enabled {
		w.WriteInt32(1)
	} else {
		w.WriteInt32(0)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameExUseSharedGroupItem builds the shared item reuse cooldown packet.
func FrameExUseSharedGroupItem(itemID, groupID int32, remainingMillis, totalMillis int) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExUseSharedGroupItem)
	w.WriteInt32(itemID)
	w.WriteInt32(groupID)
	w.WriteInt32(int32(remainingMillis / 1000))
	w.WriteInt32(int32(totalMillis / 1000))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
