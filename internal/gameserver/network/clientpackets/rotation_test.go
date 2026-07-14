package clientpackets

import "testing"

func TestDecodeStartRotating(t *testing.T) {
	payload := []byte{
		OpcodeStartRotating,
		0x00, 0x80, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
	}

	got, err := DecodeStartRotating(payload)
	if err != nil {
		t.Fatalf("DecodeStartRotating: %v", err)
	}
	if got != (StartRotating{Degree: 32768, Side: 1}) {
		t.Fatalf("DecodeStartRotating = %+v", got)
	}
}

func TestDecodeFinishRotating(t *testing.T) {
	payload := []byte{
		OpcodeFinishRotating,
		0x34, 0x12, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
	}

	got, err := DecodeFinishRotating(payload)
	if err != nil {
		t.Fatalf("DecodeFinishRotating: %v", err)
	}
	if got != (FinishRotating{Degree: 0x1234, Side: 1}) {
		t.Fatalf("DecodeFinishRotating = %+v", got)
	}
}

func TestDecodeRotatingShort(t *testing.T) {
	if _, err := DecodeStartRotating([]byte{OpcodeStartRotating, 1, 2}); err == nil {
		t.Fatal("DecodeStartRotating: want error on short payload")
	}
	if _, err := DecodeFinishRotating([]byte{OpcodeFinishRotating, 1, 2}); err == nil {
		t.Fatal("DecodeFinishRotating: want error on short payload")
	}
}
