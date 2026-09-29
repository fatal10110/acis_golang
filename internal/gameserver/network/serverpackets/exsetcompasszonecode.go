package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

const (
	CompassSiegeZone      int32 = 0x0b
	CompassPeaceZone      int32 = 0x0c
	CompassSevenSignsZone int32 = 0x0d
	CompassPvPZone        int32 = 0x0e
	CompassGeneralZone    int32 = 0x0f
)

func FrameExSetCompassZoneCode(code int32) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExSetCompassZoneCode)
	w.WriteInt32(code)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
