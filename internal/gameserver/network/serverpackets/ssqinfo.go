package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeSSQInfo is the wire opcode for SSQInfo, the seven-signs sky state
// sent right after a character slot is chosen.
const OpcodeSSQInfo = 0xf8

// regularSkyState is the sky state shown when no cabal holds the seven-signs
// seal. The seven-signs event is not modeled, so this is the only state this
// server ever reports.
const regularSkyState = 256

// FrameSSQInfo builds the SSQInfo packet as an owned frame. The seven-signs
// event is not modeled, so it always reports the regular (no-cabal) sky.
func FrameSSQInfo() wire.Frame {
	w := newFrameWriter(OpcodeSSQInfo)
	writeSSQInfo(w)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

func writeSSQInfo(w *wire.Writer) {
	w.WriteUint16(regularSkyState)
}
