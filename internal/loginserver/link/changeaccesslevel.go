package link

import "fmt"

// OpcodeChangeAccessLevel is the wire opcode for ChangeAccessLevel, a game
// server's request to change an account's access level.
const OpcodeChangeAccessLevel = 0x04

// ChangeAccessLevel is a game server's request to change an account's
// access level.
type ChangeAccessLevel struct {
	Level   int32
	Account string
}

// DecodeChangeAccessLevel parses a raw ChangeAccessLevel payload (opcode
// byte included).
func DecodeChangeAccessLevel(payload []byte) (ChangeAccessLevel, error) {
	r := newReader(payload)
	c := ChangeAccessLevel{
		Level:   r.ReadInt32(),
		Account: r.ReadString(),
	}
	if r.Err() != nil {
		return ChangeAccessLevel{}, fmt.Errorf("link: ChangeAccessLevel: %w", r.Err())
	}
	return c, nil
}
