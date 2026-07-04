package clientpackets

import "fmt"

// OpcodeAuthGameGuard is the wire opcode for AuthGameGuard, sent by the
// client immediately after connecting.
const OpcodeAuthGameGuard = 0x07

const authGameGuardSize = 20

// AuthGameGuard answers the (no-op) GameGuard challenge with the session id
// the server assigned at connect.
type AuthGameGuard struct {
	SessionID int32
	Data1     int32
	Data2     int32
	Data3     int32
	Data4     int32
}

// DecodeAuthGameGuard parses a raw AuthGameGuard payload (opcode byte
// included).
func DecodeAuthGameGuard(payload []byte) (AuthGameGuard, error) {
	r := newReader(payload)
	if r.remaining() < authGameGuardSize {
		return AuthGameGuard{}, fmt.Errorf("clientpackets: AuthGameGuard: need %d bytes, got %d", authGameGuardSize, r.remaining())
	}
	return AuthGameGuard{
		SessionID: r.readInt32(),
		Data1:     r.readInt32(),
		Data2:     r.readInt32(),
		Data3:     r.readInt32(),
		Data4:     r.readInt32(),
	}, nil
}
