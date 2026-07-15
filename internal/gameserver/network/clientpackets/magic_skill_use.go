package clientpackets

import "fmt"

const requestMagicSkillUseSize = 4 + 4 + 1

// RequestMagicSkillUse asks the server to cast one known active skill.
type RequestMagicSkillUse struct {
	SkillID      int32
	CtrlPressed  bool
	ShiftPressed bool
}

// DecodeRequestMagicSkillUse parses a raw RequestMagicSkillUse payload
// (opcode byte included).
func DecodeRequestMagicSkillUse(payload []byte) (RequestMagicSkillUse, error) {
	r := newReader(payload)
	if r.Remaining() < requestMagicSkillUseSize {
		return RequestMagicSkillUse{}, fmt.Errorf("clientpackets: RequestMagicSkillUse: need %d bytes, got %d", requestMagicSkillUseSize, r.Remaining())
	}
	req := RequestMagicSkillUse{
		SkillID:      r.ReadInt32(),
		CtrlPressed:  r.ReadInt32() != 0,
		ShiftPressed: r.ReadUint8() != 0,
	}
	if err := r.Err(); err != nil {
		return RequestMagicSkillUse{}, fmt.Errorf("clientpackets: RequestMagicSkillUse: %w", err)
	}
	return req, nil
}
