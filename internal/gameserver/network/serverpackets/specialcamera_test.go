package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
)

// Golden frame: a 2-byte little-endian length that counts itself, then
// SpecialCamera's body as the field appendix lists it: 0xc7, the object id
// and the nine shot fields, each a little-endian D. The shot carries
// negative angles so their two's-complement bytes are pinned too.
func TestSpecialCameraGolden(t *testing.T) {
	shot := event.CameraShot{
		Distance: 1100, Yaw: 210, Pitch: -5, Time: 3000, Duration: 10000,
		Turn: -13, Rise: 0, Widescreen: 1, Unknown: 1,
	}
	want := []byte{
		0x2b, 0x00, 0xc7,
		0x03, 0x02, 0x00, 0x10, // object id 0x10000203
		0x4c, 0x04, 0x00, 0x00, // distance 1100
		0xd2, 0x00, 0x00, 0x00, // yaw 210
		0xfb, 0xff, 0xff, 0xff, // pitch -5
		0xb8, 0x0b, 0x00, 0x00, // time 3000
		0x10, 0x27, 0x00, 0x00, // duration 10000
		0xf3, 0xff, 0xff, 0xff, // turn -13
		0x00, 0x00, 0x00, 0x00, // rise 0
		0x01, 0x00, 0x00, 0x00, // widescreen 1
		0x01, 0x00, 0x00, 0x00, // last field 1
	}
	if got := frameBytes(t, FrameSpecialCamera(0x10000203, shot)); !bytes.Equal(got, want) {
		t.Fatalf("SpecialCamera frame = % x, want % x", got, want)
	}
}

// Golden frame: ExPlayScene is the extended opcode 0xfe and its 2-byte
// sub-opcode 0x5b, with no body.
func TestExPlaySceneGolden(t *testing.T) {
	want := []byte{0x05, 0x00, 0xfe, 0x5b, 0x00}
	if got := frameBytes(t, FrameExPlayScene()); !bytes.Equal(got, want) {
		t.Fatalf("ExPlayScene frame = % x, want % x", got, want)
	}
}
