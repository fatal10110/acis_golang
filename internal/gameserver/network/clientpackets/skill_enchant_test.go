package clientpackets

import "testing"

func TestDecodeSkillEnchantRequests(t *testing.T) {
	info, err := DecodeRequestExEnchantSkillInfo([]byte{
		OpcodeExtended,
		0x06, 0x00,
		0x7c, 0x00, 0x00, 0x00,
		0x65, 0x00, 0x00, 0x00,
	})
	if err != nil {
		t.Fatalf("DecodeRequestExEnchantSkillInfo: %v", err)
	}
	if info != (RequestExEnchantSkillInfo{SkillID: 124, SkillLevel: 101}) {
		t.Fatalf("DecodeRequestExEnchantSkillInfo = %+v, want skill 124 level 101", info)
	}

	enchant, err := DecodeRequestExEnchantSkill([]byte{
		OpcodeExtended,
		0x07, 0x00,
		0x7d, 0x00, 0x00, 0x00,
		0x66, 0x00, 0x00, 0x00,
	})
	if err != nil {
		t.Fatalf("DecodeRequestExEnchantSkill: %v", err)
	}
	if enchant != (RequestExEnchantSkill{SkillID: 125, SkillLevel: 102}) {
		t.Fatalf("DecodeRequestExEnchantSkill = %+v, want skill 125 level 102", enchant)
	}
}

func TestDecodeSkillEnchantRequestsShort(t *testing.T) {
	if _, err := DecodeRequestExEnchantSkillInfo([]byte{OpcodeExtended, 0x06, 0x00, 1}); err == nil {
		t.Fatal("DecodeRequestExEnchantSkillInfo: want error on short payload")
	}
	if _, err := DecodeRequestExEnchantSkill([]byte{OpcodeExtended, 0x07, 0x00, 1}); err == nil {
		t.Fatal("DecodeRequestExEnchantSkill: want error on short payload")
	}
}

func TestDecodeSkillEnchantRequestsWrongExtendedOpcode(t *testing.T) {
	if _, err := DecodeRequestExEnchantSkillInfo([]byte{OpcodeExtended, 0x07, 0x00, 0, 0, 0, 0, 0, 0, 0, 0}); err == nil {
		t.Fatal("DecodeRequestExEnchantSkillInfo: want error on wrong extended opcode")
	}
	if _, err := DecodeRequestExEnchantSkill([]byte{OpcodeExtended, 0x06, 0x00, 0, 0, 0, 0, 0, 0, 0, 0}); err == nil {
		t.Fatal("DecodeRequestExEnchantSkill: want error on wrong extended opcode")
	}
}
