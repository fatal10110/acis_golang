package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestScenarioMonsterWithoutMakerTerritoryIsDriftSphere pins the spawn
// territory of a scenario monster placed with no npcmaker: the drift sphere
// (200) around its spawn point, as for a civilian, not anywhere.
func TestScenarioMonsterWithoutMakerTerritoryIsDriftSphere(t *testing.T) {
	srv := gameservertest.Boot(t)
	at := location.Location{X: 1000, Y: 2000, Z: 30}
	h := srv.SpawnHostileNPCOfMakerAt(t, &npc.Template{
		ID: probeID, TemplateID: probeID, Type: "Monster", Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}, at, nil)
	for _, tc := range []struct {
		dx   int
		want bool
	}{{0, true}, {150, true}, {240, false}, {2000, false}} {
		got := h.SpawnTerritoryContains(location.Location{X: at.X + tc.dx, Y: at.Y, Z: at.Z})
		if got != tc.want {
			t.Errorf("spawn territory contains spawn point +%d = %v, want %v", tc.dx, got, tc.want)
		}
	}
}
