package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeSSQInfo is the wire opcode for SSQInfo, the seven-signs sky state
// sent right after a character slot is chosen and on every period change.
const OpcodeSSQInfo = 0xf8

// regularSkyState is the sky state shown outside seal validation, or when
// the competition was tied.
const regularSkyState = SSQSkyRegular

// Seven-signs sky states SSQInfo carries.
const (
	SSQSkyRegular uint16 = 256
	SSQSkyDusk    uint16 = 257
	SSQSkyDawn    uint16 = 258
	SSQSkyRed     uint16 = 259
)

// FrameSSQInfo builds the SSQInfo packet showing the regular sky as an owned
// frame.
func FrameSSQInfo() wire.Frame {
	return FrameSSQInfoSky(regularSkyState)
}

// FrameSSQInfoSky builds the SSQInfo packet showing sky state as an owned
// frame.
func FrameSSQInfoSky(state uint16) wire.Frame {
	w := newFrameWriter(OpcodeSSQInfo)
	w.WriteUint16(state)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
