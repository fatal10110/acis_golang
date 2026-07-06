package serverpackets

// OpcodeCharCreateOk is the wire opcode for CharCreateOk, acknowledging a
// successful character creation.
const OpcodeCharCreateOk = 0x19

// EncodeCharCreateOk builds the CharCreateOk packet.
func EncodeCharCreateOk() []byte {
	w := newWriter(OpcodeCharCreateOk)
	w.WriteInt32(1)
	return w.Bytes()
}
