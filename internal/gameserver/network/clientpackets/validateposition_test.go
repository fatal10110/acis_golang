package clientpackets

import "testing"

func TestDecodeValidatePosition(t *testing.T) {
	payload := []byte{
		OpcodeValidatePosition,
		0x50, 0xb4, 0x00, 0x00,
		0x15, 0xa1, 0x00, 0x00,
		0x32, 0xf2, 0xff, 0xff,
		0x00, 0x80, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}

	got, err := DecodeValidatePosition(payload)
	if err != nil {
		t.Fatalf("DecodeValidatePosition: %v", err)
	}

	want := ValidatePosition{X: 46160, Y: 41237, Z: -3534, Heading: 32768}
	if got != want {
		t.Fatalf("DecodeValidatePosition = %+v, want %+v", got, want)
	}
}

func TestDecodeValidatePosition_Short(t *testing.T) {
	if _, err := DecodeValidatePosition([]byte{OpcodeValidatePosition, 1, 2}); err == nil {
		t.Fatal("DecodeValidatePosition: want error on short payload")
	}
}
