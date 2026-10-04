package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Golden frames: a 2-byte little-endian length that counts itself, then
// the body as the reference writes it. ObserverStart: 0xdf, x, y, z, yaw,
// pitch (D each). ObserverEnd: 0xe0, x, y, z (D each, writeLoc).
func TestObserverStartGolden(t *testing.T) {
	got := frameBytes(t, FrameObserverStart(location.Location{X: -18347, Y: 114000, Z: -2360}, 49152, 0))
	want := []byte{
		0x17, 0x00, 0xdf,
		0x55, 0xb8, 0xff, 0xff, // -18347
		0x50, 0xbd, 0x01, 0x00, // 114000
		0xc8, 0xf6, 0xff, 0xff, // -2360
		0x00, 0xc0, 0x00, 0x00, // 49152
		0x00, 0x00, 0x00, 0x00, // 0
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("ObserverStart frame = % x, want % x", got, want)
	}
}

func TestObserverEndGolden(t *testing.T) {
	got := frameBytes(t, FrameObserverEnd(location.Location{X: 83400, Y: 147943, Z: -3404}))
	want := []byte{
		0x0f, 0x00, 0xe0,
		0xc8, 0x45, 0x01, 0x00, // 83400
		0xe7, 0x41, 0x02, 0x00, // 147943
		0xb4, 0xf2, 0xff, 0xff, // -3404
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("ObserverEnd frame = % x, want % x", got, want)
	}
}
