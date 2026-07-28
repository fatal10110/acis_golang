package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestFrameFlyToLocation(t *testing.T) {
	got := framePayload(t, FrameFlyToLocation(
		268476516,
		location.Location{X: 46160, Y: 41237, Z: -3534},
		location.Location{X: 12345, Y: -6789, Z: 100},
		skill.FlightThrowUp,
	))
	want := []byte{
		OpcodeFlyToLocation,
		0x64, 0xa0, 0x00, 0x10,
		0x50, 0xb4, 0x00, 0x00,
		0x15, 0xa1, 0x00, 0x00,
		0x32, 0xf2, 0xff, 0xff,
		0x39, 0x30, 0x00, 0x00,
		0x7b, 0xe5, 0xff, 0xff,
		0x64, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameFlyToLocation() = %x, want %x", got, want)
	}
}
