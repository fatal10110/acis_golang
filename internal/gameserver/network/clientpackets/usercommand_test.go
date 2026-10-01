package clientpackets

import "testing"

// TestDecodeRequestUserCommand pins RequestUserCommand: opcode 0xaa and one
// little-endian int32 command id (here 0x51, PartyInfo).
func TestDecodeRequestUserCommand(t *testing.T) {
	got, err := DecodeRequestUserCommand([]byte{0xaa, 0x51, 0x00, 0x00, 0x00})
	if err != nil {
		t.Fatalf("DecodeRequestUserCommand: %v", err)
	}
	if want := (RequestUserCommand{CommandID: 81}); got != want {
		t.Errorf("DecodeRequestUserCommand = %+v, want %+v", got, want)
	}
	if _, err := DecodeRequestUserCommand([]byte{OpcodeRequestUserCommand, 0x51, 0x00, 0x00}); err == nil {
		t.Error("DecodeRequestUserCommand: want error on short payload, got nil")
	}
}
