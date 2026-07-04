package link

// OpcodeInitLS is the wire opcode for InitLS, the first packet the login
// server sends a game server once it connects.
const OpcodeInitLS = 0x00

// linkProtocolRevision is the GS-LS link protocol revision InitLS
// advertises.
const linkProtocolRevision = 0x0102

// EncodeInitLS builds the InitLS packet: the link protocol revision and the
// login server's RSA public modulus, sent in the clear (the link is still
// on its static bootstrap key at this point).
func EncodeInitLS(publicKey []byte) []byte {
	w := newWriter(OpcodeInitLS)
	w.writeInt32(linkProtocolRevision)
	w.writeInt32(int32(len(publicKey)))
	w.writeBytes(publicKey)
	return w.bytes()
}
