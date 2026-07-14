package clientpackets

import "testing"

func TestDecodeCannotMoveAnymore(t *testing.T) {
	payload := []byte{
		OpcodeCannotMoveAnymore,
		0x50, 0xb4, 0x00, 0x00,
		0x15, 0xa1, 0x00, 0x00,
		0x32, 0xf2, 0xff, 0xff,
		0x00, 0x80, 0x00, 0x00,
	}

	got, err := DecodeCannotMoveAnymore(payload)
	if err != nil {
		t.Fatalf("DecodeCannotMoveAnymore: %v", err)
	}
	want := CannotMoveAnymore{X: 46160, Y: 41237, Z: -3534, Heading: 32768}
	if got != want {
		t.Fatalf("DecodeCannotMoveAnymore = %+v, want %+v", got, want)
	}
}

func TestDecodeCannotMoveAnymoreShort(t *testing.T) {
	if _, err := DecodeCannotMoveAnymore([]byte{OpcodeCannotMoveAnymore, 1, 2}); err == nil {
		t.Fatal("DecodeCannotMoveAnymore: want error on short payload")
	}
}
