package clientpackets

import "testing"

func TestDecodeRequestMagicSkillUse(t *testing.T) {
	payload := []byte{
		OpcodeRequestMagicSkillUse,
		0x03, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x01,
	}

	got, err := DecodeRequestMagicSkillUse(payload)
	if err != nil {
		t.Fatalf("DecodeRequestMagicSkillUse: %v", err)
	}
	want := RequestMagicSkillUse{SkillID: 3, CtrlPressed: true, ShiftPressed: true}
	if got != want {
		t.Fatalf("DecodeRequestMagicSkillUse = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestMagicSkillUseShort(t *testing.T) {
	if _, err := DecodeRequestMagicSkillUse([]byte{OpcodeRequestMagicSkillUse, 1}); err == nil {
		t.Fatal("DecodeRequestMagicSkillUse: want error on short payload")
	}
}
