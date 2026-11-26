// Package clientwire encodes the client packets that every behavior suite
// sends to enter the world, so the suites share one copy of each.
package clientwire

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
)

// RequestGameStart is the RequestGameStart packet selecting character slot.
func RequestGameStart(slot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGameStart)
	w.WriteInt32(slot)
	w.WriteUint16(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	return w.Bytes()
}

// EnterWorld is the EnterWorld packet.
func EnterWorld() []byte {
	return wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes()
}

// SingleOpcode is a packet that is only its opcode.
func SingleOpcode(opcode byte) []byte {
	return wire.NewPacketWriter(opcode).Bytes()
}
