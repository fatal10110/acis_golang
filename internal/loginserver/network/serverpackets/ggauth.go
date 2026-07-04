package serverpackets

// OpcodeGGAuth is the wire opcode for GGAuth, the (no-op) GameGuard
// challenge response.
const OpcodeGGAuth = 0x0b

// GGAuthSkipRequest is the response code sent when GameGuard authentication
// is skipped entirely.
const GGAuthSkipRequest int32 = 0x0b

// EncodeGGAuth builds the GGAuth packet for response.
func EncodeGGAuth(response int32) []byte {
	w := newWriter(OpcodeGGAuth)
	w.writeInt32(response)
	w.writeInt32(0)
	w.writeInt32(0)
	w.writeInt32(0)
	w.writeInt32(0)
	return w.bytes()
}
