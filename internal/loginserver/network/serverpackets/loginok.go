package serverpackets

// OpcodeLoginOk is the wire opcode for LoginOk, sent after a successful
// login authentication.
const OpcodeLoginOk = 0x03

// EncodeLoginOk builds the LoginOk packet, carrying the session key half the
// client presents back in RequestServerList/RequestServerLogin.
func EncodeLoginOk(sessionKey1, sessionKey2 int32) []byte {
	w := newWriter(OpcodeLoginOk)
	w.writeInt32(sessionKey1)
	w.writeInt32(sessionKey2)
	w.writeInt32(0)
	w.writeInt32(0)
	w.writeInt32(0x000003ea)
	w.writeInt32(0)
	w.writeInt32(0)
	w.writeInt32(0)
	w.writeBytes(make([]byte, 16))
	return w.bytes()
}
