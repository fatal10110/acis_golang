package clientpackets

import "testing"

func TestDecodeRequestGetBossRecord(t *testing.T) {
	req, err := DecodeRequestGetBossRecord([]byte{OpcodeExtended, 0x18, 0x00, 0xa9, 0x61, 0x00, 0x00})
	if err != nil || req.BossID != 25001 {
		t.Fatalf("DecodeRequestGetBossRecord() = %+v, %v; want boss 25001", req, err)
	}
	if _, err := DecodeRequestGetBossRecord([]byte{OpcodeExtended, 0x18, 0x00, 0xa9, 0x61}); err == nil {
		t.Fatal("a short boss id decoded")
	}
	if _, err := DecodeRequestGetBossRecord([]byte{OpcodeExtended, 0x19, 0x00, 0, 0, 0, 0}); err == nil {
		t.Fatal("another sub-opcode decoded")
	}
}
