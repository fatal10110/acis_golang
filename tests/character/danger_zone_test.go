package character

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// etcStatusDangers is the danger field (the fourth int32) of every
// EtcStatusUpdate among frames, in order.
func etcStatusDangers(frames [][]byte) []int32 {
	var out []int32
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeEtcStatusUpdate {
			continue
		}
		r := wire.NewReader(f[1:])
		r.ReadInt32() // charges
		r.ReadInt32() // weight penalty
		r.ReadInt32() // message refusal
		out = append(out, r.ReadInt32())
	}
	return out
}

func assertDangers(t *testing.T, when string, frames [][]byte, want ...int32) {
	t.Helper()
	got := etcStatusDangers(frames)
	if len(got) != len(want) {
		t.Fatalf("%s: EtcStatusUpdate danger fields = %v, want %v", when, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s: EtcStatusUpdate danger fields = %v, want %v", when, got, want)
		}
	}
}

func assertInDanger(t *testing.T, when string, character *player.Character, want bool) {
	t.Helper()
	if got := character.InDangerArea(); got != want {
		t.Fatalf("%s: InDangerArea = %v, want %v", when, got, want)
	}
}

// TestDangerZonesDriveTheEtcStatusDangerField pins DamageZone and
// EffectZone onEnter/onExit (DamageZone.java:93-109, EffectZone.java:108-124):
// every entry marks the player with DANGER_AREA and sends it its
// EtcStatusUpdate, and an exit sends one only once no overlapping danger
// zone still holds it. EtcStatusUpdate writes that flag as its fourth field
// (EtcStatusUpdate.java:23), so every other one sent inside carries it too.
func TestDangerZonesDriveTheEtcStatusDangerField(t *testing.T) {
	damageForm, err := zone.NewCuboid(2_000, 4_000, -1_000, 1_000, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	damage, err := zone.NewDamage(1, damageForm, commons.NewStatSet())
	if err != nil {
		t.Fatal(err)
	}
	effectForm, err := zone.NewCuboid(3_000, 5_000, -1_000, 1_000, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	effectZone, err := zone.NewEffect(2, effectForm, commons.NewStatSet())
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	zones.Add(damage)
	zones.Add(effectZone)
	srv, character, objID := bootInZones(t, zones)
	x, y, z := srv.PlayerPosition(t, objID)
	appear(t, srv.Client)
	assertInDanger(t, "outside", character, false)

	// Into the damage zone alone.
	assertDangers(t, "into the damage zone", teleportIntoWater(t, srv, character, 2_500, y, z), 1)
	assertInDanger(t, "in the damage zone", character, true)

	// Any other EtcStatusUpdate sent inside carries the flag.
	character.IncreaseCharges(1, 2)
	assertDangers(t, "charge inside", readUntilQuiet(srv.Client), 1)

	// The teleport leaves the only danger zone; Appearing enters both
	// overlapping ones, each sending its own update.
	character.TeleportTo(3_500, y, z, 0)
	assertDangers(t, "teleport out of the damage zone", readUntilQuiet(srv.Client), 0)
	assertInDanger(t, "off the grid", character, false)
	srv.Client.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	assertDangers(t, "into both zones", readUntilQuiet(srv.Client), 1, 1)
	assertInDanger(t, "in both zones", character, true)

	// Leaving both sends one update, from the last zone to let go, not
	// one per zone; the effect zone alone then marks the player again.
	character.TeleportTo(4_500, y, z, 0)
	assertDangers(t, "teleport out of both zones", readUntilQuiet(srv.Client), 0)
	srv.Client.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	assertDangers(t, "into the effect zone", readUntilQuiet(srv.Client), 1)
	assertInDanger(t, "in the effect zone", character, true)

	// Out of every danger zone.
	character.TeleportTo(x, y, z, 0)
	assertDangers(t, "teleport out of the effect zone", readUntilQuiet(srv.Client), 0)
	srv.Client.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	assertDangers(t, "back outside", readUntilQuiet(srv.Client))
	assertInDanger(t, "back outside", character, false)
	character.IncreaseCharges(1, 2)
	assertDangers(t, "charge outside", readUntilQuiet(srv.Client), 0)
}
