package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// The grand boss teleporters (#210's fourth batch): the cubes' exits. The
// Baium and Core cube, the fire dragon gatekeepers' doors and Klein's count
// are tests/quest/testdata/scenarios/grand_boss_teleporter_*. Shared
// fixtures are in teleporters_210_pr3_test.go.

// TestGrandBossTeleporterExits: each cube's own page or talk sends the
// player out of its lair: Benom's and Sailren's to one of their points up
// to 100 away, Frintezza's up to 500 from its point, Valakas's up to 250,
// Antharas's anywhere in its 600 by 1100 field.
func TestGrandBossTeleporterExits(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		npcID  int
		page   string
		cmd    string
		offset int
		spots  []location.Location
	}{
		{
			name: "benom", npcID: tpBenomCube, page: "29055-01.htm", cmd: "Quest GrandBossTeleporter benom_exit", offset: 100,
			spots: []location.Location{{X: 11913, Y: -48851, Z: -1088}, {X: 11918, Y: -49447, Z: -1088}},
		},
		{
			name: "frintezza", npcID: tpFrintCube, page: "29061-01.htm", cmd: "Quest GrandBossTeleporter FRINTEZZA_EXIT", offset: 500,
			spots: []location.Location{{X: 150037, Y: -57720, Z: -2976}},
		},
		{name: "valakas", npcID: tpValakasCube, offset: 250, spots: []location.Location{{X: 150037, Y: -57720, Z: -2976}}},
		{
			name: "sailren", npcID: tpSailrenCube, offset: 100,
			spots: []location.Location{{X: 10610, Y: -24035, Z: -3676}, {X: 10703, Y: -24041, Z: -3673}, {X: 10769, Y: -24107, Z: -3672}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := bootOracles(t, oracleSetup{})
			cube := w.spawnFolk(t, folkTemplate("Folk", tt.npcID), 50)
			if tt.page != "" {
				w.selectFolk(t, cube)
				wantTalkPage(t, w.talk(t, cube, false), cube, teleportPage(t, cube, "GrandBossTeleporter", tt.page))
			}
			w.openAnyNpcPage(t)
			cmd := tt.cmd
			if cmd == "" {
				cmd = npcCommand(cube, "Quest GrandBossTeleporter")
			}
			w.requireTripNear(t, w.tripFrames(t, cmd), tt.offset, tt.spots...)
		})
	}
	t.Run("antharas", func(t *testing.T) {
		t.Parallel()
		w := bootOracles(t, oracleSetup{})
		cube := w.spawnFolk(t, folkTemplate("Folk", tpAntharasCub), 50)
		w.openThrough(t, cube)
		got := w.landedAt(t, w.tripFrames(t, npcCommand(cube, "Quest GrandBossTeleporter")))
		if got.X < 79800 || got.X >= 80400 || got.Y < 151200 || got.Y >= 152300 || got.Z != -3534 {
			t.Fatalf("landed at %+v, want inside [79800,80400) x [151200,152300) at -3534", got)
		}
	})
}
