package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// The expected bytes below are written out from the reference writers, not
// from the encoders under test: ShowCalculator writeC(0xdc) writeD(id);
// ShowMiniMap writeC(0x9d) writeD(mapId) writeD(period ordinal);
// ShowXMasSeal writeC(0xf2) writeD(item); Dice writeC(0xd4) writeD(objectId)
// writeD(itemId) writeD(number) then writeLoc's x, y, z.

func TestFrameShowCalculator(t *testing.T) {
	got := framePayload(t, FrameShowCalculator(4393))
	want := []byte{0xdc, 0x29, 0x11, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameShowCalculator(4393) = %x, want %x", got, want)
	}
}

func TestFrameShowMiniMap(t *testing.T) {
	got := framePayload(t, FrameShowMiniMap(RegularMapID, 3))
	want := []byte{0x9d, 0x81, 0x06, 0x00, 0x00, 0x03, 0x00, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameShowMiniMap(1665, 3) = %x, want %x", got, want)
	}
}

func TestFrameShowXMasSeal(t *testing.T) {
	got := framePayload(t, FrameShowXMasSeal(5555))
	want := []byte{0xf2, 0xb3, 0x15, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameShowXMasSeal(5555) = %x, want %x", got, want)
	}
}

func TestFrameDice(t *testing.T) {
	got := framePayload(t, FrameDice(0x10203040, 4625, 6, location.Location{X: -84318, Y: 244579, Z: -3730}))
	want := []byte{
		0xd4,
		0x40, 0x30, 0x20, 0x10, // roller object id
		0x11, 0x12, 0x00, 0x00, // 4625
		0x06, 0x00, 0x00, 0x00, // number
		0xa2, 0xb6, 0xfe, 0xff, // -84318
		0x63, 0xbb, 0x03, 0x00, // 244579
		0x6e, 0xf1, 0xff, 0xff, // -3730
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameDice() = %x, want %x", got, want)
	}
}

// TestDiceLocationAheadOfRoller pins the spot a die lands: the roller's
// position moved 30 units along its heading, each axis truncated toward
// zero (SpawnLocation.addOffsetBasedOnHeading: heading / 182.0444… degrees,
// Math.toRadians, x += (int)(cos*30), y += (int)(sin*30), z kept). The
// expected offsets were computed independently with the same double
// arithmetic in Python, not with the Go helper.
func TestDiceLocationAheadOfRoller(t *testing.T) {
	from := location.Location{X: 100, Y: -200, Z: 300}
	for _, tt := range []struct {
		heading, dx, dy int
	}{
		{0, 30, 0},
		{8192, 21, 21},
		{12345, 11, 27},
		{16384, 0, 30},
		{32768, -30, 0},
		{40000, -23, -19},
		{49152, 0, -30},
		{60000, 25, -15},
		{65535, 29, 0},
	} {
		got := location.OrientedLocation{Location: from, Heading: tt.heading}.Ahead(30)
		want := location.Location{X: from.X + tt.dx, Y: from.Y + tt.dy, Z: from.Z}
		if got != want {
			t.Errorf("heading %d: Ahead(30) = %+v, want %+v", tt.heading, got, want)
		}
	}
}
