package clientpackets

import "fmt"

const requestExMagicSkillUseGroundSize = 2 + 4 + 4 + 4 + 4 + 4 + 1

// RequestExMagicSkillUseGround asks the server to cast one known ground-
// targeted active skill (e.g. a signet) at a client-supplied world point.
type RequestExMagicSkillUseGround struct {
	X, Y, Z      int32
	SkillID      int32
	CtrlPressed  bool
	ShiftPressed bool
}

// DecodeRequestExMagicSkillUseGround parses a raw extended
// RequestExMagicSkillUseGround payload (opcode byte included).
func DecodeRequestExMagicSkillUseGround(payload []byte) (RequestExMagicSkillUseGround, error) {
	r, err := newExtendedReader(payload, "RequestExMagicSkillUseGround", OpcodeRequestExMagicSkillUseGround, requestExMagicSkillUseGroundSize)
	if err != nil {
		return RequestExMagicSkillUseGround{}, err
	}
	req := RequestExMagicSkillUseGround{
		X:            r.ReadInt32(),
		Y:            r.ReadInt32(),
		Z:            r.ReadInt32(),
		SkillID:      r.ReadInt32(),
		CtrlPressed:  r.ReadInt32() != 0,
		ShiftPressed: r.ReadUint8() != 0,
	}
	if err := r.Err(); err != nil {
		return RequestExMagicSkillUseGround{}, fmt.Errorf("clientpackets: RequestExMagicSkillUseGround: %w", err)
	}
	return req, nil
}
