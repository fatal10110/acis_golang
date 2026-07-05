package serverpackets

// OpcodeLoginOk is the wire opcode for LoginOk, sent after a successful
// login authentication.
const OpcodeLoginOk = 0x03

// EncodeLoginOk builds the LoginOk packet, carrying the session key half the
// client presents back in RequestServerList/RequestServerLogin.
func EncodeLoginOk(sessionKey1, sessionKey2 int32) []byte {
	w := newWriter(OpcodeLoginOk)
	w.WriteInt32(sessionKey1)
	w.WriteInt32(sessionKey2)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0x000003ea)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteBytes(make([]byte, 16))
	return w.Bytes()
}
