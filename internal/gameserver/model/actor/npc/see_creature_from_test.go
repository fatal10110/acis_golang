package npc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// TestForceSeeCreatureFromInstantRows replays the instant-teleport rows of
// the teleport re-fire golden through ForceSeeCreatureFrom, the player
// having jumped from a point of the NPC's own region.
func TestForceSeeCreatureFromInstantRows(t *testing.T) {
	for _, r := range scriptcontract.Lookup(t, "aggro.teleport_see").Rows {
		if r.Str(t, "entry") != "instantTeleportTo" {
			continue
		}
		t.Run(r.ID, func(t *testing.T) {
			w, n := newSightNPC(false, false)
			c := sightTarget(t, w, r, r.Bool(t, "in_range"), int(r.Int(t, "dz")))
			ForceSeeCreatureFrom(w, c, location.Location{X: sightX - 300, Y: sightY})
			if got, want := len(n.told), int(r.Int(t, "calls")); got != want {
				t.Fatalf("calls = %d, want %d", got, want)
			}
			if n.sight.hasSeen(c) {
				t.Fatal("the re-fire filled the NPC's seen set")
			}
		})
	}
}

// TestForceSeeCreatureFromScansThePreJumpRegion pins the scan's origin
// (golden aggro.teleport_see, note on instantTeleportTo): the NPCs scanned
// are those around the region the player jumped from, while the range is
// measured from where it landed. A player landing in an NPC's sight from
// three regions away tells it nothing; one landing there from the region
// next to the NPC's tells it once.
func TestForceSeeCreatureFromScansThePreJumpRegion(t *testing.T) {
	const regionSize = 2048
	for _, tc := range []struct {
		name  string
		fromX int
		calls int
	}{
		{"from three regions away", sightX + 3*regionSize, 0},
		{"from the next region", sightX + regionSize, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, n := newSightNPC(false, false)
			c := &sightCreature{id: 2, kind: actor.KindPlayer}
			w.Spawn(c, sightX+100, sightY, 0, 0)
			ForceSeeCreatureFrom(w, c, location.Location{X: tc.fromX, Y: sightY})
			if len(n.told) != tc.calls {
				t.Fatalf("calls = %v, want %d", n.told, tc.calls)
			}
		})
	}
}

// TestForceSeeCreatureFromOffTheGrid pins that a creature off the grid
// tells nobody, whatever point it jumped from.
func TestForceSeeCreatureFromOffTheGrid(t *testing.T) {
	w, n := newSightNPC(false, false)
	c := &sightCreature{id: 2, kind: actor.KindPlayer}
	ForceSeeCreatureFrom(w, c, location.Location{X: sightX, Y: sightY})
	if len(n.told) != 0 {
		t.Fatalf("calls = %v, want none off the grid", n.told)
	}
}
