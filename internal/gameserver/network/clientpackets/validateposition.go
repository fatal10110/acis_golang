package clientpackets

import "fmt"

// OpcodeValidatePosition is the wire opcode for the client's periodic
// position report.
const OpcodeValidatePosition = 0x48

const validatePositionSize = 5 * 4

// ValidatePosition reports the client's current character position.
type ValidatePosition struct {
	X, Y, Z int32
	Heading int32
	BoatID  int32
}

// DecodeValidatePosition parses a raw ValidatePosition payload (opcode byte
// included).
func DecodeValidatePosition(payload []byte) (ValidatePosition, error) {
	r := newReader(payload)
	if r.Remaining() < validatePositionSize {
		return ValidatePosition{}, fmt.Errorf("clientpackets: ValidatePosition: need %d bytes, got %d", validatePositionSize, r.Remaining())
	}
	req := ValidatePosition{
		X:       r.ReadInt32(),
		Y:       r.ReadInt32(),
		Z:       r.ReadInt32(),
		Heading: r.ReadInt32(),
		BoatID:  r.ReadInt32(),
	}
	if err := r.Err(); err != nil {
		return ValidatePosition{}, fmt.Errorf("clientpackets: ValidatePosition: %w", err)
	}
	return req, nil
}
