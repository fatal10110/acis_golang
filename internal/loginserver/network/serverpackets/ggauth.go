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
	w.WriteInt32(response)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	return w.Bytes()
}
