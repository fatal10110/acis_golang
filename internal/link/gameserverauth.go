package link

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// OpcodeGameServerAuth is the wire opcode for GameServerAuth, the game
// server's registration/authentication request.
const OpcodeGameServerAuth = 0x01

// GameServerAuth is a game server's request to register (or re-authenticate)
// on this login server: the server id it wants, whether it accepts an
// alternate id or a shared host reservation if that id is taken, its
// host/port/capacity, and its hex auth key.
type GameServerAuth struct {
	DesiredID         byte
	AcceptAlternateID bool
	HostReserved      bool
	HostName          string
	Port              uint16
	MaxPlayers        int32
	HexID             []byte
}

// DecodeGameServerAuth parses a raw GameServerAuth payload (opcode byte
// included).
func DecodeGameServerAuth(payload []byte) (GameServerAuth, error) {
	r := newReader(payload)
	auth := GameServerAuth{
		DesiredID:         r.ReadUint8(),
		AcceptAlternateID: r.ReadUint8() != 0,
		HostReserved:      r.ReadUint8() != 0,
		HostName:          r.ReadString(),
		Port:              r.ReadUint16(),
		MaxPlayers:        r.ReadInt32(),
	}
	size := int(r.ReadInt32())
	auth.HexID = r.ReadBytes(size)
	if r.Err() != nil {
		return GameServerAuth{}, fmt.Errorf("link: GameServerAuth: %w", r.Err())
	}
	return auth, nil
}

// EncodeGameServerAuth builds the GameServerAuth packet a game server sends
// to register (or re-authenticate) on this login server.
func EncodeGameServerAuth(auth GameServerAuth) []byte {
	w := newWriter(OpcodeGameServerAuth)
	w.WriteUint8(auth.DesiredID)
	w.WriteUint8(wire.BoolByte(auth.AcceptAlternateID))
	w.WriteUint8(wire.BoolByte(auth.HostReserved))
	w.WriteString(auth.HostName)
	w.WriteUint16(auth.Port)
	w.WriteInt32(auth.MaxPlayers)
	w.WriteInt32(int32(len(auth.HexID)))
	w.WriteBytes(auth.HexID)
	return w.Bytes()
}
