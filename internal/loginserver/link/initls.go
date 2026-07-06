package link

import "fmt"

// OpcodeInitLS is the wire opcode for InitLS, the first packet the login
// server sends a game server once it connects.
const OpcodeInitLS = 0x00

// ProtocolRevision is the GS-LS link protocol revision InitLS advertises; a
// game server must refuse to proceed with the handshake if the login
// server's revision does not match this one.
const ProtocolRevision = 0x0102

// EncodeInitLS builds the InitLS packet: the link protocol revision and the
// login server's RSA public modulus, sent in the clear (the link is still
// on its static bootstrap key at this point).
func EncodeInitLS(publicKey []byte) []byte {
	w := newWriter(OpcodeInitLS)
	w.WriteInt32(ProtocolRevision)
	w.WriteInt32(int32(len(publicKey)))
	w.WriteBytes(publicKey)
	return w.Bytes()
}

// DecodeInitLS parses a raw InitLS payload (opcode byte included) into the
// link protocol revision it advertises and the login server's raw RSA
// public modulus bytes.
func DecodeInitLS(payload []byte) (revision int32, publicKey []byte, err error) {
	r := newReader(payload)
	revision = r.ReadInt32()
	size := int(r.ReadInt32())
	publicKey = r.ReadBytes(size)
	if r.Err() != nil {
		return 0, nil, fmt.Errorf("link: InitLS: %w", r.Err())
	}
	return revision, publicKey, nil
}
