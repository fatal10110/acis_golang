package link

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// newWriter starts a GS-LS link packet with its opcode byte.
func newWriter(opcode byte) *wire.Writer {
	w := &wire.Writer{}
	w.WriteUint8(opcode)
	return w
}

func boolByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}
