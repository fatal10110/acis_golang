package link

import "fmt"

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

// String returns a human-readable description of reason, for logging.
func (reason LoginServerFailReason) String() string {
	switch reason {
	case ReasonIPBanned:
		return "ip banned"
	case ReasonIPReserved:
		return "ip reserved"
	case ReasonWrongHexID:
		return "wrong hexid"
	case ReasonIDReserved:
		return "id reserved"
	case ReasonNoFreeID:
		return "no free ID"
	case ReasonNotAuthed:
		return "not authed"
	case ReasonAlreadyLoggedIn:
		return "already logged in"
	default:
		return fmt.Sprintf("unknown reason %d", byte(reason))
	}
}

// EncodeLoginServerFail builds the LoginServerFail packet for reason.
func EncodeLoginServerFail(reason LoginServerFailReason) []byte {
	w := newWriter(OpcodeLoginServerFail)
	w.WriteUint8(byte(reason))
	return w.Bytes()
}

// DecodeLoginServerFail parses a raw LoginServerFail payload (opcode byte
// included) into its reason code.
func DecodeLoginServerFail(payload []byte) (LoginServerFailReason, error) {
	r := newReader(payload)
	reason := LoginServerFailReason(r.ReadUint8())
	if r.Err() != nil {
		return 0, fmt.Errorf("link: LoginServerFail: %w", r.Err())
	}
	return reason, nil
}
