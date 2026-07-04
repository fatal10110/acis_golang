package link

// OpcodeLoginServerFail is the wire opcode for LoginServerFail, sent to
// refuse a game server's connection or registration.
const OpcodeLoginServerFail = 0x01

// LoginServerFailReason is the reason code sent in a LoginServerFail
// packet.
type LoginServerFailReason byte

// LoginServerFail reasons.
const (
	ReasonIPBanned        LoginServerFailReason = 1
	ReasonIPReserved      LoginServerFailReason = 2
	ReasonWrongHexID      LoginServerFailReason = 3
	ReasonIDReserved      LoginServerFailReason = 4
	ReasonNoFreeID        LoginServerFailReason = 5
	ReasonNotAuthed       LoginServerFailReason = 6
	ReasonAlreadyLoggedIn LoginServerFailReason = 7
)

// EncodeLoginServerFail builds the LoginServerFail packet for reason.
func EncodeLoginServerFail(reason LoginServerFailReason) []byte {
	w := newWriter(OpcodeLoginServerFail)
	w.writeByte(byte(reason))
	return w.bytes()
}
