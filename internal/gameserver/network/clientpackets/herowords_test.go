package clientpackets

import "testing"

func TestDecodeRequestWriteHeroWords(t *testing.T) {
	// "Hi" as null-terminated UTF-16LE after the extended opcode 0x000c.
	req, err := DecodeRequestWriteHeroWords([]byte{OpcodeExtended, 0x0c, 0x00, 'H', 0, 'i', 0, 0, 0})
	if err != nil || req.Message != "Hi" {
		t.Fatalf("DecodeRequestWriteHeroWords() = %+v, %v; want message Hi", req, err)
	}
	if _, err := DecodeRequestWriteHeroWords([]byte{OpcodeExtended, 0x0d, 0x00, 'H', 0, 0, 0}); err == nil {
		t.Fatal("DecodeRequestWriteHeroWords() accepted another sub-opcode")
	}
}
