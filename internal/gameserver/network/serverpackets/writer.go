package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// newWriter starts a game server packet with its opcode byte.
func newWriter(opcode byte) *wire.Writer {
	w := &wire.Writer{}
	w.WriteUint8(opcode)
	return w
}

// boolByte encodes b as the wire's 1/0 byte convention.
func boolByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}
