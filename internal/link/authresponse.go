package link

import "fmt"

// OpcodeAuthResponse is the wire opcode for AuthResponse, accepting a game
// server's registration.
const OpcodeAuthResponse = 0x02

// EncodeAuthResponse builds the AuthResponse packet, confirming the
// server id the game server was assigned and its registered name.
func EncodeAuthResponse(serverID byte, serverName string) []byte {
	w := newWriter(OpcodeAuthResponse)
	w.WriteUint8(serverID)
	w.WriteString(serverName)
	return w.Bytes()
}

// DecodeAuthResponse parses a raw AuthResponse payload (opcode byte
// included) into the assigned server id and registered name.
func DecodeAuthResponse(payload []byte) (serverID byte, serverName string, err error) {
	r := newReader(payload)
	serverID = r.ReadUint8()
	serverName = r.ReadString()
	if r.Err() != nil {
		return 0, "", fmt.Errorf("link: AuthResponse: %w", r.Err())
	}
	return serverID, serverName, nil
}
