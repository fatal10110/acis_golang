package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func TestFrameAutoAttackStart(t *testing.T) {
	got := framePayload(t, FrameAutoAttackStart(12345))
	want := []byte{OpcodeAutoAttackStart, 0x39, 0x30, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameAutoAttackStart() = %x, want %x", got, want)
	}
}

func TestFrameSocialAction(t *testing.T) {
	got := framePayload(t, FrameSocialAction(12345, 13))
	want := []byte{
		OpcodeSocialAction,
		0x39, 0x30, 0x00, 0x00,
		0x0d, 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameSocialAction() = %x, want %x", got, want)
	}
}

func TestFrameChangeMoveType(t *testing.T) {
	got := framePayload(t, FrameChangeMoveType(12345, false, false))
	want := []byte{
		OpcodeChangeMoveType,
		0x39, 0x30, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameChangeMoveType() = %x, want %x", got, want)
	}
}

func TestFrameChangeWaitType(t *testing.T) {
	got := framePayload(t, FrameChangeWaitType(12345, WaitSitting, location.Location{X: 46160, Y: 41237, Z: -3534}))
	want := []byte{
		OpcodeChangeWaitType,
		0x39, 0x30, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x50, 0xb4, 0x00, 0x00,
		0x15, 0xa1, 0x00, 0x00,
		0x32, 0xf2, 0xff, 0xff,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameChangeWaitType() = %x, want %x", got, want)
	}
}
