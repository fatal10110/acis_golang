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
