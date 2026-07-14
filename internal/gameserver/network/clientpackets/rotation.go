package clientpackets

import "fmt"

const rotatingSize = 2 * 4

// StartRotating reports the beginning of client-side character rotation.
type StartRotating struct {
	Degree int32
	Side   int32
}

// DecodeStartRotating parses a raw start-rotation payload (opcode byte
// included).
func DecodeStartRotating(payload []byte) (StartRotating, error) {
	r := newReader(payload)
	if r.Remaining() < rotatingSize {
		return StartRotating{}, fmt.Errorf("clientpackets: StartRotating: need %d bytes, got %d", rotatingSize, r.Remaining())
	}
	req := StartRotating{
		Degree: r.ReadInt32(),
		Side:   r.ReadInt32(),
	}
	if err := r.Err(); err != nil {
		return StartRotating{}, fmt.Errorf("clientpackets: StartRotating: %w", err)
	}
	return req, nil
}

// FinishRotating reports the final client-side character heading.
type FinishRotating struct {
	Degree int32
	Side   int32
}

// DecodeFinishRotating parses a raw finish-rotation payload (opcode byte
// included).
func DecodeFinishRotating(payload []byte) (FinishRotating, error) {
	r := newReader(payload)
	if r.Remaining() < rotatingSize {
		return FinishRotating{}, fmt.Errorf("clientpackets: FinishRotating: need %d bytes, got %d", rotatingSize, r.Remaining())
	}
	req := FinishRotating{
		Degree: r.ReadInt32(),
		Side:   r.ReadInt32(),
	}
	if err := r.Err(); err != nil {
		return FinishRotating{}, fmt.Errorf("clientpackets: FinishRotating: %w", err)
	}
	return req, nil
}
