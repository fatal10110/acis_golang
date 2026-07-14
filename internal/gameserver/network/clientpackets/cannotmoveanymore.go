package clientpackets

import "fmt"

const cannotMoveAnymoreSize = 4 * 4

// CannotMoveAnymore reports the client-side location where movement stopped.
type CannotMoveAnymore struct {
	X, Y, Z int32
	Heading int32
}

// DecodeCannotMoveAnymore parses a raw movement-stop payload (opcode byte
// included).
func DecodeCannotMoveAnymore(payload []byte) (CannotMoveAnymore, error) {
	r := newReader(payload)
	if r.Remaining() < cannotMoveAnymoreSize {
		return CannotMoveAnymore{}, fmt.Errorf("clientpackets: CannotMoveAnymore: need %d bytes, got %d", cannotMoveAnymoreSize, r.Remaining())
	}
	req := CannotMoveAnymore{
		X:       r.ReadInt32(),
		Y:       r.ReadInt32(),
		Z:       r.ReadInt32(),
		Heading: r.ReadInt32(),
	}
	if err := r.Err(); err != nil {
		return CannotMoveAnymore{}, fmt.Errorf("clientpackets: CannotMoveAnymore: %w", err)
	}
	return req, nil
}
