package link

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
