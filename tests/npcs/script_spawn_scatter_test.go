package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// wallGeo is sinkingGeo with a wall around every point: a move from a point
// to a target ends where it started.
type wallGeo struct{ sinkingGeo }

func (wallGeo) ValidLocation(x, y, z, _, _, _ int) location.Location {
	return location.Location{X: x, Y: y, Z: z}
}

// wolvesOf returns where each wolf in the world stands.
func wolvesOf(w *scriptSpawnWorld) []location.Location {
	var out []location.Location
	for _, obj := range w.srv.State.Objects() {
		if h, ok := obj.(*npc.Hostile); ok && h.Instance.Template.ID == spawnWolfID {
			x, y, z := h.Position()
			out = append(out, location.Location{X: x, Y: y, Z: z})
		}
	}
	return out
}

// A script spawn with a random offset really moves the NPC: over many
// spawns it lands off the asked point on both axes and to both sides of
// it, never more than 100 away on an axis, and always on the ground from
// 20 above where geodata let it reach.
func TestScriptSpawnRandomOffsetScatters(t *testing.T) {
	t.Parallel()
	w := bootScriptSpawns(t)
	x, y, z := w.at.X+60, w.at.Y, w.at.Z
	const spawns = 60
	for range spawns {
		if h := w.spawner.AddSpawn(spawnWolfID, script.Loc{X: x, Y: y, Z: z}, true, 0); h == nil {
			t.Fatal("scattered spawn returned no handle")
		}
	}
	at := wolvesOf(w)
	if len(at) != spawns {
		t.Fatalf("wolves in the world = %d, want %d", len(at), spawns)
	}
	var movedX, movedY, west, east, north, south bool
	for _, l := range at {
		dx, dy := l.X-x, l.Y-y
		if dx < -100 || dx > 100 || dy < -100 || dy > 100 {
			t.Fatalf("wolf at %d,%d; want within 100 of %d,%d", l.X, l.Y, x, y)
		}
		// The offset point's ground is z-5; 20 above it, the ground is 5 lower still.
		if l.Z != z+20-2*groundBelow {
			t.Fatalf("wolf at z %d, want %d", l.Z, z+20-2*groundBelow)
		}
		movedX, movedY = movedX || dx != 0, movedY || dy != 0
		west, east = west || dx < 0, east || dx > 0
		north, south = north || dy < 0, south || dy > 0
	}
	if !movedX || !movedY || !west || !east || !north || !south {
		t.Errorf("60 scattered spawns around %d,%d did not spread on both axes to both sides: x moved %v y moved %v, west %v east %v, north %v south %v",
			x, y, movedX, movedY, west, east, north, south)
	}
}

// Where geodata allows no move, a scattered spawn stays at the asked
// point; one without the offset does the same.
func TestScriptSpawnRandomOffsetStopsAtWall(t *testing.T) {
	t.Parallel()
	w := bootScriptSpawns(t, gameservertest.WithGeo(wallGeo{}))
	x, y, z := w.at.X+60, w.at.Y, w.at.Z
	for range 10 {
		if h := w.spawner.AddSpawn(spawnWolfID, script.Loc{X: x, Y: y, Z: z}, true, 0); h == nil {
			t.Fatal("scattered spawn returned no handle")
		}
	}
	at := wolvesOf(w)
	if len(at) != 10 {
		t.Fatalf("wolves in the world = %d, want 10", len(at))
	}
	for _, l := range at {
		if l.X != x || l.Y != y {
			t.Fatalf("wolf at %d,%d behind a wall, want it at %d,%d", l.X, l.Y, x, y)
		}
	}
}
