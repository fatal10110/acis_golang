package character

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// awaitHPLoss lets the clock run a millisecond at a time until the player
// loses HP, for at most limit, and checks it lost exactly want. Client
// reads move the driven clock too, so this finds the pulse instead of
// predicting it.
func awaitHPLoss(t *testing.T, srv *gameservertest.Server, objID int32, when string, limit time.Duration, want int) {
	t.Helper()
	for passed := time.Duration(0); passed < limit; passed += time.Millisecond {
		before := srv.PlayerCurrentHP(t, objID)
		srv.Advance(t, time.Millisecond)
		if lost := before - srv.PlayerCurrentHP(t, objID); lost > 0 {
			if lost != want {
				t.Fatalf("%s: HP lost = %d, want %d", when, lost, want)
			}
			return
		}
	}
	t.Fatalf("%s: no HP lost within %v", when, limit)
}

// assertHPLossAt lets the clock run to just before the damage zone's next
// pulse, after from now, then over it, and checks the pulse took exactly
// want HP: the pulse runs at its fixed rate.
func assertHPLossAt(t *testing.T, srv *gameservertest.Server, objID int32, when string, after time.Duration, want int) {
	t.Helper()
	srv.Advance(t, after-time.Millisecond)
	before := srv.PlayerCurrentHP(t, objID)
	srv.Advance(t, time.Millisecond)
	if got := before - srv.PlayerCurrentHP(t, objID); got != want {
		t.Fatalf("%s: HP lost = %d, want %d", when, got, want)
	}
}

// TestDamageZoneHurtsAPlayerEveryReuseDelay pins DamageZone's task
// (DamageZone.java:63-90): the first entry schedules it at initialDelay,
// then every reuseDelay; each pulse takes hpDamage (times 1 +
// DAMAGE_ZONE_VULN / 100, 0 here) from the living player inside. Leaving
// ends the damage, and the empty zone's task stops: a new entry starts a
// fresh one, at initialDelay from that entry.
func TestDamageZoneHurtsAPlayerEveryReuseDelay(t *testing.T) {
	t.Parallel()
	form, err := zone.NewCuboid(2_000, 4_000, -1_000, 1_000, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	set := commons.NewStatSet()
	set.Set("hpDamage", "5")
	set.Set("initialDelay", "1100")
	set.Set("reuseDelay", "5300")
	damage, err := zone.NewDamage(1, form, set)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	zones.Add(damage)
	srv, character, objID := bootInZones(t, zones)
	if !srv.DrivesClock() {
		t.Skip("pulse timing needs the driven clock")
	}
	x, y, z := srv.PlayerPosition(t, objID)
	appear(t, srv.Client)

	teleportIntoWater(t, srv, character, 3_000, y, z)
	assertOccupant(t, damage, objID)
	awaitHPLoss(t, srv, objID, "first pulse", 1100*time.Millisecond, 5)
	assertHPLossAt(t, srv, objID, "second pulse", 5300*time.Millisecond, 5)
	assertHPLossAt(t, srv, objID, "third pulse", 5300*time.Millisecond, 5)

	// Out of the zone: no more damage, whatever the clock does.
	teleportIntoWater(t, srv, character, x, y, z)
	before := srv.PlayerCurrentHP(t, objID)
	srv.Advance(t, 12*time.Second)
	if after := srv.PlayerCurrentHP(t, objID); after < before {
		t.Fatalf("HP outside the zone went from %d to %d", before, after)
	}

	// Back in: a fresh task, within initialDelay of the entry.
	teleportIntoWater(t, srv, character, 3_000, y, z)
	awaitHPLoss(t, srv, objID, "first pulse after re-entry", 1100*time.Millisecond, 5)
	assertHPLossAt(t, srv, objID, "second pulse after re-entry", 5300*time.Millisecond, 5)

	// The pulses go on until one kills the player: a death with no killer.
	srv.Advance(t, 2*5300*time.Millisecond)
	if !character.Dead() {
		t.Fatal("player not dead after two more pulses took its last 10 HP")
	}
	if hp := srv.PlayerCurrentHP(t, objID); hp != 0 {
		t.Fatalf("HP of the player the zone killed = %d, want 0", hp)
	}
}

// assertOccupant fails unless objID is among z's occupants.
func assertOccupant(t *testing.T, z *zone.Damage, objID int32) {
	t.Helper()
	for _, a := range z.Occupants() {
		if a.ObjectID() == objID {
			return
		}
	}
	t.Fatalf("object %d is not an occupant of zone %d", objID, z.ID())
}
