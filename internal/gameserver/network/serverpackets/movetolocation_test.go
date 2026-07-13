package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func TestFrameMoveToLocation(t *testing.T) {
	got := framePayload(t, FrameMoveToLocation(
		268476516,
		location.Location{X: 46160, Y: 41237, Z: -3534},
		location.Location{X: 46117, Y: 41247, Z: -3532},
	))
	want := []byte{
		0x01,
		0x64, 0xa0, 0x00, 0x10,
		0x50, 0xb4, 0x00, 0x00,
		0x15, 0xa1, 0x00, 0x00,
		0x32, 0xf2, 0xff, 0xff,
		0x25, 0xb4, 0x00, 0x00,
		0x1f, 0xa1, 0x00, 0x00,
		0x34, 0xf2, 0xff, 0xff,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameMoveToLocation() = %x, want %x", got, want)
	}
}
