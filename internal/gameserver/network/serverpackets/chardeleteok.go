package serverpackets

// OpcodeCharDeleteOk is the wire opcode for CharDeleteOk, acknowledging a
// successful character deletion request.
const OpcodeCharDeleteOk = 0x23

// EncodeCharDeleteOk builds the CharDeleteOk packet.
func EncodeCharDeleteOk() []byte {
	return newWriter(OpcodeCharDeleteOk).Bytes()
}
