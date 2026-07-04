package serverpackets

// OpcodePlayOk is the wire opcode for PlayOk, sent after a client selects a
// game server it may play on.
const OpcodePlayOk = 0x07

// EncodePlayOk builds the PlayOk packet, carrying the session key half the
// client presents to the chosen game server at EnterWorld.
func EncodePlayOk(sessionKey1, sessionKey2 int32) []byte {
	w := newWriter(OpcodePlayOk)
	w.writeInt32(sessionKey1)
	w.writeInt32(sessionKey2)
	return w.bytes()
}
