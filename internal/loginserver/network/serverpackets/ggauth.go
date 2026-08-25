package serverpackets

// OpcodeGGAuth is the wire opcode for GGAuth, the (no-op) GameGuard
// challenge response.
const OpcodeGGAuth = 0x0b

// EncodeGGAuth builds the GGAuth packet for response, carrying back the
// connection's Init session id.
func EncodeGGAuth(response int32) []byte {
	w := newWriter(OpcodeGGAuth)
	w.WriteInt32(response)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	return w.Bytes()
}
