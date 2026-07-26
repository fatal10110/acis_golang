package clientpackets

import "testing"

func TestDecodeRequestExMagicSkillUseGround(t *testing.T) {
	payload := []byte{
		OpcodeExtended,
		0x2f, 0x00,
		0x10, 0x00, 0x00, 0x00,
		0x20, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x03, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x01,
	}

	got, err := DecodeRequestExMagicSkillUseGround(payload)
	if err != nil {
		t.Fatalf("DecodeRequestExMagicSkillUseGround: %v", err)
	}
	want := RequestExMagicSkillUseGround{X: 16, Y: 32, Z: 0, SkillID: 3, CtrlPressed: true, ShiftPressed: true}
	if got != want {
		t.Fatalf("DecodeRequestExMagicSkillUseGround = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestExMagicSkillUseGroundShort(t *testing.T) {
	if _, err := DecodeRequestExMagicSkillUseGround([]byte{OpcodeExtended, 0x2f, 0x00, 1}); err == nil {
		t.Fatal("DecodeRequestExMagicSkillUseGround: want error on short payload")
	}
}

func TestDecodeRequestExMagicSkillUseGroundWrongOpcode(t *testing.T) {
	payload := []byte{
		OpcodeExtended,
		0x10, 0x00,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	}
	if _, err := DecodeRequestExMagicSkillUseGround(payload); err == nil {
		t.Fatal("DecodeRequestExMagicSkillUseGround: want error on wrong extended opcode")
	}
}
