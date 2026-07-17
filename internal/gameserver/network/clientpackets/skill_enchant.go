package clientpackets

import "fmt"

const requestExEnchantSkillSize = 2 + 2*4

// RequestExEnchantSkillInfo asks for the costs and chance for one skill
// enchant level.
type RequestExEnchantSkillInfo struct {
	SkillID    int32
	SkillLevel int32
}

// DecodeRequestExEnchantSkillInfo parses a raw extended
// RequestExEnchantSkillInfo payload (opcode byte included).
func DecodeRequestExEnchantSkillInfo(payload []byte) (RequestExEnchantSkillInfo, error) {
	r, err := newExtendedReader(payload, "RequestExEnchantSkillInfo", OpcodeRequestExEnchantSkillInfo, requestExEnchantSkillSize)
	if err != nil {
		return RequestExEnchantSkillInfo{}, err
	}
	req := RequestExEnchantSkillInfo{
		SkillID:    r.ReadInt32(),
		SkillLevel: r.ReadInt32(),
	}
	if err := r.Err(); err != nil {
		return RequestExEnchantSkillInfo{}, fmt.Errorf("clientpackets: RequestExEnchantSkillInfo: %w", err)
	}
	return req, nil
}

// RequestExEnchantSkill asks to apply one skill enchant level.
type RequestExEnchantSkill struct {
	SkillID    int32
	SkillLevel int32
}

// DecodeRequestExEnchantSkill parses a raw extended RequestExEnchantSkill
// payload (opcode byte included).
func DecodeRequestExEnchantSkill(payload []byte) (RequestExEnchantSkill, error) {
	r, err := newExtendedReader(payload, "RequestExEnchantSkill", OpcodeRequestExEnchantSkill, requestExEnchantSkillSize)
	if err != nil {
		return RequestExEnchantSkill{}, err
	}
	req := RequestExEnchantSkill{
		SkillID:    r.ReadInt32(),
		SkillLevel: r.ReadInt32(),
	}
	if err := r.Err(); err != nil {
		return RequestExEnchantSkill{}, fmt.Errorf("clientpackets: RequestExEnchantSkill: %w", err)
	}
	return req, nil
}
