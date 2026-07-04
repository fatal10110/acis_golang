package link

// OpcodePlayerAuthResponse is the wire opcode for PlayerAuthResponse,
// answering a game server's PlayerAuthRequest.
const OpcodePlayerAuthResponse = 0x03

// EncodePlayerAuthResponse builds the PlayerAuthResponse packet, telling
// the game server whether account's presented session keys were valid.
func EncodePlayerAuthResponse(account string, ok bool) []byte {
	w := newWriter(OpcodePlayerAuthResponse)
	w.writeString(account)
	w.writeByte(boolByte(ok))
	return w.bytes()
}
