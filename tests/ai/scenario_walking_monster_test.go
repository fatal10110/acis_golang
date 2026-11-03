package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestScenarioWalkingMonsterOfMaker pins what a scenario's walking monster
// is beyond its walk (probe_walking_monster_cursed walks one): its spawn
// point is its home, and an npcmaker's NPC keeps the maker's territory as
// its spawn territory, not the drift sphere.
func TestScenarioWalkingMonsterOfMaker(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t)
	at := location.Location{X: 1000, Y: 2000, Z: 30}
	territory, err := spawn.NewTerritory("camp", at.Z-100, at.Z+100, []spawn.Node{
		{X: at.X - 100, Y: at.Y - 300}, {X: at.X + 700, Y: at.Y - 300}, {X: at.X + 700, Y: at.Y + 300}, {X: at.X - 100, Y: at.Y + 300},
	})
	if err != nil {
		t.Fatal(err)
	}
	maker := spawn.NewMaker(spawn.Maker{Name: "camp", Territories: []*spawn.Territory{territory}, AIType: "default_maker", MaximumNPCs: 1}, 1)
	h := srv.SpawnMovingHostileNPCOfMakerAt(t, &npc.Template{
		ID: probeID, TemplateID: probeID, Type: "Monster", Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20, CanMove: true,
	}, at, maker)

	if !h.Instance.HasHome || h.Instance.Home != at {
		t.Fatalf("home = %+v (%v), want the spawn point %+v", h.Instance.Home, h.Instance.HasHome, at)
	}
	for _, tc := range []struct {
		dx   int
		want bool
	}{{0, true}, {600, true}, {800, false}} {
		if got := h.SpawnTerritoryContains(location.Location{X: at.X + tc.dx, Y: at.Y, Z: at.Z}); got != tc.want {
			t.Errorf("spawn territory contains spawn point +%d = %v, want %v", tc.dx, got, tc.want)
		}
	}

	if !h.Instance.Template.CanMove {
		t.Fatal("the walking monster's template cannot move")
	}
}
