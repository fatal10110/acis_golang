package link

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// newWriter starts a GS-LS link packet with its opcode byte.
func newWriter(opcode byte) *wire.Writer {
	return wire.NewPacketWriter(opcode)
}

func boolByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}
