package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeSSQInfo is the wire opcode for SSQInfo, the seven-signs sky state
// sent right after a character slot is chosen.
const OpcodeSSQInfo = 0xf8

// regularSkyState is the sky state shown when no cabal holds the seven-signs
// seal. The seven-signs event is not modeled, so this is the only state the
// character-selection SSQInfo reports.
const regularSkyState = SSQSkyRegular

// Seven-signs sky states SSQInfo carries.
const (
	SSQSkyRegular uint16 = 256
	SSQSkyDusk    uint16 = 257
	SSQSkyDawn    uint16 = 258
	SSQSkyRed     uint16 = 259
)

// FrameSSQInfo builds the SSQInfo packet as an owned frame. The seven-signs
// event is not modeled, so it always reports the regular (no-cabal) sky.
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
