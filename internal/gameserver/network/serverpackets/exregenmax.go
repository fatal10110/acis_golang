package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

const regenMaxScale = 0.66

// FrameExRegenMax updates the client's heal-over-time regen gauge.
func FrameExRegenMax(count, period int32, hpRegen float64) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExRegenMax)
	w.WriteInt32(1)
	w.WriteInt32(count)
	w.WriteInt32(period)
	w.WriteFloat64(hpRegen * regenMaxScale)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
