package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// townRestartPoint is the table's regular restart destination.
var townRestartPoint = location.Location{X: 200, Y: 400, Z: -300}

// chaosRestartPoint is the table's karma-only restart destination.
var chaosRestartPoint = location.Location{X: -9000, Y: -9000, Z: -300}

// restartTable is a one-point table covering the class spawn region: regular
// restarts land at the town point, karma'd restarts at the chaos point.
func restartTable() *restart.Table {
	return &restart.Table{
		Points: []restart.Point{{
			Name:       "TestTown",
			Points:     []location.Location{townRestartPoint},
			ChaoPoints: []location.Location{chaosRestartPoint},
			MapRegions: []location.Point{mapRegionAt(playerOrigin)},
		}},
	}
}

// mapRegionAt converts a world position to its region-scale map coordinate,
// mirroring restart.Table's own grouping.
func mapRegionAt(loc location.Location) location.Point {
	return location.Point{
		X: (loc.X-world.MinX)/world.TileSize + world.TileXMin,
		Y: (loc.Y-world.MinY)/world.TileSize + world.TileYMin,
	}
}

// assertRestartTeleport reads frames until the revive broadcast and the
// teleport destination frame arrive, and asserts the destination sits within
// the restart scatter radius of want.
func assertRestartTeleport(t *testing.T, c *scriptedClient, objectID int32, want location.Location) {
	t.Helper()
	var dest location.Location
	for i := 0; i < 50; i++ {
		frame := mustRead(t, c, "revive/teleport frame")
		switch frame[0] {
		case serverpackets.OpcodeRevive:
			if got := wireReader(frame[1:]).ReadInt32(); got != objectID {
				t.Fatalf("Revive object id = %d, want %d", got, objectID)
			}
		case serverpackets.OpcodeTeleportToLocation:
			r := wireReader(frame[1:])
			if got := r.ReadInt32(); got != objectID {
				t.Fatalf("TeleportToLocation object id = %d, want %d", got, objectID)
			}
			dest = location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
			if dx := abs(dest.X - want.X); dx > 25 || abs(dest.Y-want.Y) > 25 {
				t.Fatalf("teleport destination = (%d,%d,%d), want within scatter of (%d,%d)",
					dest.X, dest.Y, dest.Z, want.X, want.Y)
			}
			return
		}
	}
	t.Fatal("TeleportToLocation never arrived after restart")
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// TestRestartPointRevivesDeadPlayer walks the death-recovery flow: a dead
// player selecting a restart point is revived (revive broadcast on the wire),
// teleported to the nearest town point, and stands back up alive.
func TestRestartPointRevivesDeadPlayer(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithRestartPoints(restartTable()),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)

	srv.MarkPlayerDead(t, objID)
	c.Send(encodeRequestRestartPoint(0))
	assertRestartTeleport(t, c, objID, townRestartPoint)

	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state after restart")
	}
	dead, ok := obj.(interface{ Dead() bool })
	if !ok {
		t.Fatalf("world player %T does not expose Dead()", obj)
	}
	if dead.Dead() {
		t.Fatal("player still dead after restart-point revive")
	}
}

// TestKarmaRestartsAtChaosPoint pins the karma branch of the same flow: a
// karma-positive player's restart resolves to the chaotic list.
func TestKarmaRestartsAtChaosPoint(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithWantChars(1),
		gameservertest.WithSeed(seedKarmaCharacter(240)),
		gameservertest.WithRestartPoints(restartTable()),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)

	srv.MarkPlayerDead(t, objID)
	c.Send(encodeRequestRestartPoint(0))
	assertRestartTeleport(t, c, objID, chaosRestartPoint)
}
