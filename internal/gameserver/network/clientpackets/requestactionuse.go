package clientpackets

import "fmt"

const requestActionUseSize = 4 + 4 + 1

// RequestActionUse is an owner-issued action-bar command.
type RequestActionUse struct {
	ActionID     int32
	CtrlPressed  bool
	ShiftPressed bool
}

// DecodeRequestActionUse parses a raw action-use request payload (opcode
// byte included).
func DecodeRequestActionUse(payload []byte) (RequestActionUse, error) {
	r := newReader(payload)
	if r.Remaining() < requestActionUseSize {
		return RequestActionUse{}, fmt.Errorf("clientpackets: RequestActionUse: need %d bytes, got %d", requestActionUseSize, r.Remaining())
	}
	req := RequestActionUse{
		ActionID:     r.ReadInt32(),
		CtrlPressed:  r.ReadInt32() == 1,
		ShiftPressed: r.ReadUint8() == 1,
	}
	if err := r.Err(); err != nil {
		return RequestActionUse{}, fmt.Errorf("clientpackets: RequestActionUse: %w", err)
	}
	return req, nil
}
