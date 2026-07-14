package serverpackets

import (
	"bytes"
	"testing"
)

func TestFrameExAutoSoulShot(t *testing.T) {
	got := framePayload(t, FrameExAutoSoulShot(1463, true))
	want := []byte{
		OpcodeExtended,
		0x12, 0x00,
		0xb7, 0x05, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExAutoSoulShot() = %x, want %x", got, want)
	}
}
