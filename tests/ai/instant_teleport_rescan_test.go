package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// regionEdge is the edge length of one world region.
const regionEdge = 2048

// TestInstantTeleportTwiceTellsTheMonstersOfTheKeptRegion
// (Creature.instantTeleportTo, forceSeeCreature over the knownlist of the
// region kept): a character jumps three regions away, then jumps again
// before any position update. It is still held in the region it started
// in, so the second jump tells the monsters around that region that see
// its landing, and none around the point it jumped from: a
// WarriorAggressive monster just inside the start's block of regions
// wants to attack it with 200, one just outside it does not.
func TestInstantTeleportTwiceTellsTheMonstersOfTheKeptRegion(t *testing.T) {
	t.Parallel()
	w := bootWarriors(t, warriorTemplate(aggressiveID, nil), warriorTemplate(aggressiveTwoID, nil))
	// edge is where the second region east of the start's begins: the
	// first one outside its block.
	edge := world.MinX + ((w.at.X-world.MinX)/regionEdge+2)*regionEdge
	inside := w.spawnAt(t, aggressiveID, location.Location{X: edge - 250, Y: w.at.Y, Z: w.at.Z})
	outside := w.spawnAt(t, aggressiveTwoID, location.Location{X: edge + 50, Y: w.at.Y, Z: w.at.Z})

	w.instantTeleport(t, location.Location{X: edge + regionEdge + 952, Y: w.at.Y, Z: w.at.Z})
	if inside.AggroHate(w.player) != 0 || outside.AggroHate(w.player) != 0 {
		t.Fatalf("hate after the first jump = %v, %v, want none: neither sees the landing",
			inside.AggroHate(w.player), outside.AggroHate(w.player))
	}

	w.instantTeleport(t, location.Location{X: edge - 100, Y: w.at.Y, Z: w.at.Z})
	if got := inside.AggroHate(w.player); got != 200 {
		t.Fatalf("hate of the monster around the kept region = %v, want 200", got)
	}
	if got := outside.AggroHate(w.player); got != 0 {
		t.Fatalf("hate of the monster around the point jumped from = %v, want none", got)
	}
}
