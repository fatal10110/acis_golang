package boat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The sea the Rune boat lies in for TestPassengerStepHeldAtWaterSurface:
// its surface 110 above the boat, its floor far deeper than 20 below that.
const (
	waterSurface = -3500
	seabed       = -4500
)

// TestPassengerStepHeldAtWaterSurface: a passenger's own server-side steps
// are held at the surface of the water zone it stands in, swimming or not,
// as a flyer's are (PlayerMove.updatePosition: canBypassZCheck holds for a
// player whose boat is set). A packet-boarded passenger clicks a folk NPC
// standing above the sea; the approach climbs toward it but never past the
// surface.
func TestPassengerStepHeldAtWaterSurface(t *testing.T) {
	t.Parallel()
	// The sea covers the boat and the water west of it, not the boarding
	// point east of it.
	form, err := zone.NewCuboid(33_800, 34_420, -37_800, -37_500, -5_000, waterSurface)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewWater(1, form))
	srv, c, me, b := bootPassengerWith(t, runeBoarding, 1, nil, gameservertest.WithZones(zones), gameservertest.WithGeo(gameservertest.FlatGeo{Z: seabed}))
	board(t, srv, c, b)
	if x, y, z := srv.PlayerPosition(t, me); (location.Location{X: x, Y: y, Z: z}) != runeDock {
		t.Fatalf("passenger at %d,%d,%d, want the boat's %v", x, y, z, runeDock)
	}

	above := location.Location{X: runeDock.X - 300, Y: runeDock.Y, Z: -3_300}
	folk := srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", 30001), above)
	c.Send(encodeAction(folk.ObjectID(), false)) // select
	srv.ReadQueued(t, c)
	c.Send(encodeAction(folk.ObjectID(), false)) // interact: approach
	srv.ReadQueued(t, c)

	mover := srv.PlayerMove(t, me)
	if !mover.Moving() {
		t.Fatal("passenger not approaching the folk NPC")
	}
	// The climb reaches the surface within a few seconds of steps...
	reached := 0
	for i := 1; i <= 100 && reached == 0; i++ {
		srv.TickPositions()
		if _, _, z := srv.PlayerPosition(t, me); z > waterSurface {
			t.Fatalf("tick %d: passenger at height %d, above the water surface %d", i, z, waterSurface)
		} else if z == waterSurface {
			reached = i
		}
	}
	if reached == 0 {
		t.Fatalf("passenger never climbed to the water surface %d", waterSurface)
	}
	// ...and stays there while the approach goes on west.
	lastX, _, _ := srv.PlayerPosition(t, me)
	for i := range 10 {
		srv.TickPositions()
		x, _, z := srv.PlayerPosition(t, me)
		if z != waterSurface || x >= lastX || !mover.Moving() {
			t.Fatalf("tick %d past the surface: passenger at x=%d height %d (moving %v), want still stepping west of %d at height %d", reached+i+1, x, z, mover.Moving(), lastX, waterSurface)
		}
		lastX = x
	}
}
