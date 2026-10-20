package npcs

import (
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The grand boss teleporters (#210's fourth batch): the cubes' exits and
// despawns, the fire dragon gatekeepers' doors and Klein's count. Shared
// fixtures are in teleporters_210_pr3_test.go.

// baiumZone is a grand boss zone of the Baium lair's id around the point
// dx east of the fixture spawn point, too small to hold the player.
func baiumZone(t *testing.T, dx int) *zone.Index {
	t.Helper()
	at := gameservertest.ClassTemplate().Spawns[0]
	form, err := zone.NewCuboid(at.X+dx-20, at.X+dx+20, at.Y-20, at.Y+20, at.Z-100, at.Z+100)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewBoss(tpBaiumZone, form))
	return zones
}

// TestGrandBossTeleporterCubesLeaveAfterTheirTime: Benom's cube leaves the
// world twenty minutes after it appears, every other cube fifteen.
func TestGrandBossTeleporterCubesLeaveAfterTheirTime(t *testing.T) {
	t.Parallel()
	w := bootOracles(t, oracleSetup{zones: baiumZone(t, 1000)})
	ids := []int{tpBenomCube, tpFrintCube, tpBaiumCube, tpAntharasCub, tpValakasCube, tpSailrenCube}
	for i, id := range ids {
		// A fixture spawner runs no created hook: run it as the spawn pass does.
		w.srv.RunCreated(t, w.spawnFolk(t, folkTemplate("Folk", id), 40+10*i))
	}
	w.srv.Settle(t)
	count := func(step string, want map[int]int) {
		t.Helper()
		for _, id := range ids {
			if got := liveCount(w, id); got != want[id] {
				t.Fatalf("%s: cubes %d in the world = %d, want %d", step, id, got, want[id])
			}
		}
	}
	all := map[int]int{}
	for _, id := range ids {
		all[id] = 1
	}
	w.srv.Advance(t, 15*time.Minute-time.Millisecond)
	count("before fifteen minutes", all)
	w.srv.Advance(t, time.Millisecond)
	count("after fifteen minutes", map[int]int{tpBenomCube: 1})
	w.srv.Advance(t, 5*time.Minute-time.Millisecond)
	count("before twenty minutes", map[int]int{tpBenomCube: 1})
	w.srv.Advance(t, time.Millisecond)
	count("after twenty minutes", nil)
}

// TestGrandBossTeleporterExits: each cube's own page or talk sends the
// player out of its lair: Benom's and Sailren's to one of their points up
// to 100 away, Frintezza's up to 500 from its point, Valakas's up to 250,
// Antharas's anywhere in its 600 by 1100 field; the Baium and Core cube to
// one of Baium's points up to 100 away inside Baium's lair, to one of
// Core's points elsewhere.
func TestGrandBossTeleporterExits(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		npcID  int
		page   string
		cmd    string
		zoneDX int
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
		{
			name: "baium", npcID: tpBaiumCube, page: "31842-01.htm", zoneDX: 50, offset: 100,
			spots: []location.Location{{X: 108784, Y: 16000, Z: -4928}, {X: 113824, Y: 10448, Z: -5164}, {X: 115488, Y: 22096, Z: -5168}},
		},
		{
			name: "core", npcID: tpBaiumCube, page: "31842-01.htm", zoneDX: 1000,
			spots: []location.Location{{X: 17252, Y: 114121, Z: -3439}, {X: 17253, Y: 114232, Z: -3439}},
		},
		{name: "valakas", npcID: tpValakasCube, offset: 250, spots: []location.Location{{X: 150037, Y: -57720, Z: -2976}}},
		{
			name: "sailren", npcID: tpSailrenCube, offset: 100,
			spots: []location.Location{{X: 10610, Y: -24035, Z: -3676}, {X: 10703, Y: -24041, Z: -3673}, {X: 10769, Y: -24107, Z: -3672}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			zoneDX := tt.zoneDX
			if zoneDX == 0 {
				zoneDX = 1000
			}
			w := bootOracles(t, oracleSetup{zones: baiumZone(t, zoneDX)})
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

// TestGrandBossTeleporterFireDragonGatekeepersOpenDoors: each gatekeeper
// of the fire dragon opens its own door, answering no page.
func TestGrandBossTeleporterFireDragonGatekeepersOpenDoors(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		npcID int
		door  int
	}{
		{npcID: tpFireGK1, door: 24210004},
		{npcID: tpFireGK2, door: 24210006},
		{npcID: tpFireGK3, door: 24210005},
	} {
		t.Run(strconv.Itoa(tt.npcID), func(t *testing.T) {
			t.Parallel()
			doors := []int{24210004, 24210005, 24210006}
			w := bootOracles(t, oracleSetup{doors: []*door.Template{
				scenarioDoor(doors[0], 200), scenarioDoor(doors[1], 240), scenarioDoor(doors[2], 280),
			}})
			gk := w.spawnFolk(t, folkTemplate("Folk", tt.npcID), 50)
			w.openThrough(t, gk)
			frames := w.tripFrames(t, npcCommand(gk, "Quest GrandBossTeleporter"))
			if _, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage); ok {
				t.Fatalf("the gatekeeper answered a page: %x", opcodes(frames))
			}
			if got := w.openDoors(t, doors...); len(got) != 1 || got[0] != tt.door {
				t.Fatalf("open doors = %v, want [%d]", got, tt.door)
			}
		})
	}
}

// TestGrandBossTeleporterKleinCountsEntries: Klein takes a floating stone
// and sends its holder into the Lair of Valakas, refusing a player without
// one with 31540-06.htm. His page follows how many players he sent in:
// 31540-01.htm below 50, 31540-02.htm from 50 on, until Valakas dies and
// the count starts over.
func TestGrandBossTeleporterKleinCountsEntries(t *testing.T) {
	t.Parallel()
	w := bootOracles(t, oracleSetup{held: map[int32]int32{tpFloatingStne: 50}})
	klein := w.spawnFolk(t, folkTemplate("Folk", tpKlein), 50)
	w.openThrough(t, klein)
	wantTalkAnswer(t, w.tripFrames(t, npcCommand(klein, "Quest GrandBossTeleporter")), klein,
		teleportPage(t, klein, "GrandBossTeleporter", "31540-01.htm"))
	w.requireTrip(t, w.tripFrames(t, "Quest GrandBossTeleporter 31540"), tpValakasIn)

	// A Klein inside the lair, so the player can come back to him.
	klein = w.spawnFolk(t, folkTemplate("Folk", tpKlein), 50)
	w.openThrough(t, klein)
	for i := 2; i <= 49; i++ {
		w.requireTrip(t, w.tripFrames(t, "Quest GrandBossTeleporter 31540"), tpValakasIn)
	}
	w.openThrough(t, klein)
	wantTalkAnswer(t, w.tripFrames(t, npcCommand(klein, "Quest GrandBossTeleporter")), klein,
		teleportPage(t, klein, "GrandBossTeleporter", "31540-01.htm"))
	w.requireTrip(t, w.tripFrames(t, "Quest GrandBossTeleporter 31540"), tpValakasIn)
	w.requireSaved(t, map[int32]int{tpFloatingStne: 0})

	w.openThrough(t, klein)
	wantTalkAnswer(t, w.tripFrames(t, npcCommand(klein, "Quest GrandBossTeleporter")), klein,
		teleportPage(t, klein, "GrandBossTeleporter", "31540-02.htm"))
	frames := w.tripFrames(t, "Quest GrandBossTeleporter 31540")
	w.requireNoTrip(t, frames)
	wantPage(t, frames, klein, teleportPage(t, klein, "GrandBossTeleporter", "31540-06.htm"), 1)

	valakas := w.srv.SpawnHostileNPCTemplateAt(t, raidTemplate(tpValakas), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
	obj, ok := w.srv.State.Player(w.player)
	if !ok {
		t.Fatal("the character is not in the world")
	}
	done := make(chan struct{})
	w.srv.PlayerQueue(t, w.player).Post(func() {
		defer close(done)
		valakas.TakeDamage(valakas.MaxHP()+1, obj.(attackable.Combatant))
	})
	<-done
	w.srv.Advance(t, 3*time.Second)
	drainFrames(t, w.c)
	w.openThrough(t, klein)
	wantTalkAnswer(t, w.tripFrames(t, npcCommand(klein, "Quest GrandBossTeleporter")), klein,
		teleportPage(t, klein, "GrandBossTeleporter", "31540-01.htm"))
}
