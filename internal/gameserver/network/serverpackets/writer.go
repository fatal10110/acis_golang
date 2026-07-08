package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// newWriter starts a game server packet with its opcode byte.
func newWriter(opcode byte) *wire.Writer {
	return wire.NewPacketWriter(opcode)
}
