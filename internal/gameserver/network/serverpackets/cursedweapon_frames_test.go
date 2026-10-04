package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Reference: ExCursedWeaponLocation.writeImpl writes each weapon's id, its
// activated flag and writeLoc (x, y, z); Earthquake.writeImpl writes 0xc4,
// writeLoc, intensity, duration and the NPC flag; SystemMessage writes a
// zone-name parameter as type 7 followed by writeLoc.

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

func TestFrameEarthquake(t *testing.T) {
	got := framePayload(t, FrameEarthquake(10, -20, 30, 14, 3, false))
	want := []byte{OpcodeEarthquake}
	for _, d := range []int32{10, -20, 30, 14, 3, 0} {
		want = appendD(want, d)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameEarthquake() = %x, want %x", got, want)
	}
	if got := framePayload(t, FrameEarthquake(0, 0, 0, 1, 1, true)); got[len(got)-4] != 1 {
		t.Fatalf("FrameEarthquake(npc) flag = %x, want 1", got[len(got)-4:])
	}
}

func TestFrameSystemMessageZoneNameParam(t *testing.T) {
	got := framePayload(t, FrameSystemMessageParams(SystemMessageS2WasDroppedInTheS1Region, ZoneNameParam(100, -200, 300), ItemNameParam(8190)))
	want := []byte{OpcodeSystemMessage}
	for _, d := range []int32{1815, 2, 7, 100, -200, 300, 3, 8190} {
		want = appendD(want, d)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameSystemMessageParams(zone, item) = %x, want %x", got, want)
	}
}
