package link

import "fmt"

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
		DesiredID:         r.readByte(),
		AcceptAlternateID: r.readByte() != 0,
		HostReserved:      r.readByte() != 0,
		HostName:          r.readString(),
		Port:              r.readUint16(),
		MaxPlayers:        r.readInt32(),
	}
	size := int(r.readInt32())
	auth.HexID = r.readBytes(size)
	if r.err != nil {
		return GameServerAuth{}, fmt.Errorf("link: GameServerAuth: %w", r.err)
	}
	return auth, nil
}
