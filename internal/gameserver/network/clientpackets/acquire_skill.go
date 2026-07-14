package clientpackets

import "fmt"

const requestAcquireSkillSize = 3 * 4

// RequestAcquireSkillInfo asks for the cost and requirements of one skill
// trainer entry.
type RequestAcquireSkillInfo struct {
	SkillID   int32
	Level     int32
	SkillType int32
}

// RequestAcquireSkill asks the server to learn one skill trainer entry.
type RequestAcquireSkill struct {
	SkillID   int32
	Level     int32
	SkillType int32
}

// DecodeRequestAcquireSkillInfo parses a raw RequestAcquireSkillInfo payload
// (opcode byte included).
func DecodeRequestAcquireSkillInfo(payload []byte) (RequestAcquireSkillInfo, error) {
	r := newReader(payload)
	if r.Remaining() < requestAcquireSkillSize {
		return RequestAcquireSkillInfo{}, fmt.Errorf("clientpackets: RequestAcquireSkillInfo: need %d bytes, got %d", requestAcquireSkillSize, r.Remaining())
	}
	req := RequestAcquireSkillInfo{
		SkillID:   r.ReadInt32(),
		Level:     r.ReadInt32(),
		SkillType: r.ReadInt32(),
	}
	if err := r.Err(); err != nil {
		return RequestAcquireSkillInfo{}, fmt.Errorf("clientpackets: RequestAcquireSkillInfo: %w", err)
	}
	return req, nil
}

// DecodeRequestAcquireSkill parses a raw RequestAcquireSkill payload (opcode
// byte included).
func DecodeRequestAcquireSkill(payload []byte) (RequestAcquireSkill, error) {
	r := newReader(payload)
	if r.Remaining() < requestAcquireSkillSize {
		return RequestAcquireSkill{}, fmt.Errorf("clientpackets: RequestAcquireSkill: need %d bytes, got %d", requestAcquireSkillSize, r.Remaining())
	}
	req := RequestAcquireSkill{
		SkillID:   r.ReadInt32(),
		Level:     r.ReadInt32(),
		SkillType: r.ReadInt32(),
	}
	if err := r.Err(); err != nil {
		return RequestAcquireSkill{}, fmt.Errorf("clientpackets: RequestAcquireSkill: %w", err)
	}
	return req, nil
}
