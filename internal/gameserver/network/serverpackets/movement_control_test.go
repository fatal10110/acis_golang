package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func TestFrameStopMove(t *testing.T) {
	got := framePayload(t, FrameStopMove(268476516, location.Location{X: 46160, Y: 41237, Z: -3534}, 32768))
	want := []byte{
		OpcodeStopMove,
		0x64, 0xa0, 0x00, 0x10,
		0x50, 0xb4, 0x00, 0x00,
		0x15, 0xa1, 0x00, 0x00,
		0x32, 0xf2, 0xff, 0xff,
		0x00, 0x80, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameStopMove() = %x, want %x", got, want)
	}
}

func TestFrameStartRotation(t *testing.T) {
	got := framePayload(t, FrameStartRotation(268476516, 32768, 1, 0))
	want := []byte{
		OpcodeStartRotation,
		0x64, 0xa0, 0x00, 0x10,
		0x00, 0x80, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameStartRotation() = %x, want %x", got, want)
	}
}

func TestFrameStopRotation(t *testing.T) {
	got := framePayload(t, FrameStopRotation(268476516, 0x1234, 0))
	want := []byte{
		OpcodeStopRotation,
		0x64, 0xa0, 0x00, 0x10,
		0x34, 0x12, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x34,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameStopRotation() = %x, want %x", got, want)
	}
}
