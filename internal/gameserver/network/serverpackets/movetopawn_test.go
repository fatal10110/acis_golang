package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func TestFrameMoveToPawn(t *testing.T) {
	got := framePayload(t, FrameMoveToPawn(268476516, 268480061, 70, location.Location{X: -71440, Y: 258000, Z: -3104}))
	want := []byte{
		0x60,
		0x64, 0xa0, 0x00, 0x10,
		0x3d, 0xae, 0x00, 0x10,
		0x46, 0x00, 0x00, 0x00,
		0xf0, 0xe8, 0xfe, 0xff,
		0xd0, 0xef, 0x03, 0x00,
		0xe0, 0xf3, 0xff, 0xff,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameMoveToPawn() = %x, want %x", got, want)
	}
}
