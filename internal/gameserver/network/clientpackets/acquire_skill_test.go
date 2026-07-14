package clientpackets

import "testing"

func TestDecodeRequestAcquireSkillInfo(t *testing.T) {
	payload := []byte{
		OpcodeRequestAcquireSkillInfo,
		0x03, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}

	got, err := DecodeRequestAcquireSkillInfo(payload)
	if err != nil {
		t.Fatalf("DecodeRequestAcquireSkillInfo: %v", err)
	}
	want := RequestAcquireSkillInfo{SkillID: 3, Level: 1, SkillType: 0}
	if got != want {
		t.Fatalf("DecodeRequestAcquireSkillInfo = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestAcquireSkill(t *testing.T) {
	payload := []byte{
		OpcodeRequestAcquireSkill,
		0xf8, 0x00, 0x00, 0x00,
		0x02, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}

	got, err := DecodeRequestAcquireSkill(payload)
	if err != nil {
		t.Fatalf("DecodeRequestAcquireSkill: %v", err)
	}
	want := RequestAcquireSkill{SkillID: 248, Level: 2, SkillType: 0}
	if got != want {
		t.Fatalf("DecodeRequestAcquireSkill = %+v, want %+v", got, want)
	}
}

func TestDecodeAcquireSkillShort(t *testing.T) {
	if _, err := DecodeRequestAcquireSkillInfo([]byte{OpcodeRequestAcquireSkillInfo, 1}); err == nil {
		t.Fatal("DecodeRequestAcquireSkillInfo: want error on short payload")
	}
	if _, err := DecodeRequestAcquireSkill([]byte{OpcodeRequestAcquireSkill, 1}); err == nil {
		t.Fatal("DecodeRequestAcquireSkill: want error on short payload")
	}
}
