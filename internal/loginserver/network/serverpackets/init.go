package serverpackets

// OpcodeInit is the wire opcode for Init, the first packet the server sends
// after a client connects.
const OpcodeInit = 0x00

// protocolVersion is the Interlude client protocol revision Init advertises.
const protocolVersion = 0x0000c621

// gameGuardPlaceholder is sent in place of a real GameGuard seed; the login
// server never issues one.
var gameGuardPlaceholder [16]byte

// EncodeInit builds the Init packet: the session id, protocol revision, the
// session's scrambled RSA modulus, a GameGuard placeholder, and the
// session's Blowfish key.
func EncodeInit(sessionID int32, scrambledModulus, blowfishKey []byte) []byte {
	w := newWriter(OpcodeInit)
	w.WriteInt32(sessionID)
	w.WriteInt32(protocolVersion)
	w.WriteBytes(scrambledModulus)
	w.WriteBytes(gameGuardPlaceholder[:])
	w.WriteBytes(blowfishKey)
	w.WriteUint8(0x00)
	return w.Bytes()
}
