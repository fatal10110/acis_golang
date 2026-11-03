package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Reference: ExCursedWeaponLocation.writeImpl writes each weapon's id, its
// activated flag and writeLoc (x, y, z); Earthquake.writeImpl writes 0xc4,
// writeLoc, intensity, duration and the NPC flag.

func TestFrameExCursedWeaponLocationEntries(t *testing.T) {
	got := framePayload(t, FrameExCursedWeaponLocation([]CursedWeaponLocation{
		{ItemID: 8689, Active: true, Location: location.Location{X: 1, Y: -2, Z: 3}},
		{ItemID: 8190, Location: location.Location{X: -4, Y: 5, Z: -6}},
	}))
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExCursedWeaponLocation)
	want = appendD(want, 2)
	for _, d := range []int32{8689, 1, 1, -2, 3, 8190, 0, -4, 5, -6} {
		want = appendD(want, d)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExCursedWeaponLocation() = %x, want %x", got, want)
	}
}
