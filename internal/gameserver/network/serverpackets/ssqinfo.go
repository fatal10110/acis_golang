package serverpackets

// OpcodeSSQInfo is the wire opcode for SSQInfo, the seven-signs sky state
// sent right after a character slot is chosen.
const OpcodeSSQInfo = 0xf8

// regularSkyState is the sky state shown when no cabal holds the seven-signs
// seal. The seven-signs event is not modeled, so this is the only state this
// server ever reports.
const regularSkyState = 256

// EncodeSSQInfo builds the SSQInfo packet. The seven-signs event is not
// modeled, so it always reports the regular (no-cabal) sky.
func EncodeSSQInfo() []byte {
	w := newWriter(OpcodeSSQInfo)
	w.WriteUint16(regularSkyState)
	return w.Bytes()
}
