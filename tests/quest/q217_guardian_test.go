package quest

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q217"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Testimony of Trust's guardians (Q217_TestimonyOfTrust.onMyDying): at
// cond 2 a dryad may call Actea of Verdant Wilds and a lirein Luell of
// Zephyr Winds, unless the player already holds what the guardian gives.
// The scenarios cannot play the call: a scenario cannot show a
// script-spawned NPC appearing or leaving (#4020 item 5), and the spawn's
// random offset is not a script draw here while the reference probe pins
// it as one. These Boot tests pin it.
const (
	q217Name          = "Q217_TestimonyOfTrust"
	q217Path          = "quest.Q217_TestimonyOfTrust"
	q217Dryad         = 20013
	q217Lirein        = 20036
	q217Actea         = 27121
	q217Luell         = 27120
	q217BreathOfWinds = 2746
	q217SeedOfVerdure = 2747

	q217BeforeBattle = "S PlaySound type=0 file=Itemsound.quest_before_battle bind=0 obj=0 loc=0,0,0 delay=0"
)

// q217World boots a level 38 character whose Testimony of Trust is at cond
// 2, holding hold (0 for nothing). The guardian's roll of [0, 100) draws
// roll.
func q217World(t *testing.T, hold int32, roll int) *dialogWorld {
	t.Helper()
	w := bootSeamQuest(t, 38, nil, func(srv *gameservertest.Server, objID int32) {
		insertJournal(t, srv.DB,
			journalRow{objID, q217Name, "<state>", val("STARTED")},
			journalRow{objID, q217Name, "<cond>", val("2")})
		if hold != 0 {
			srv.GiveItem(t, objID, hold, 1)
		}
	},
		x1Items([]int32{q217BreathOfWinds, q217SeedOfVerdure}),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			proofMonster(q217Dryad), proofMonster(q217Lirein), proofMonster(q217Actea), proofMonster(q217Luell),
		})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{
				q217Dryad: script.KindHostile, q217Lirein: script.KindHostile,
				q217Actea: script.KindHostile, q217Luell: script.KindHostile,
			},
			[]script.Listing{{Path: q217Path}},
			script.Catalog{q217Path: q217.New},
		),
		gameservertest.WithScriptRand(func(n int) int {
			if n == 100 {
				return roll
			}
			return 0
		}),
	)
	w.roles[w.player] = "player"
	return w
}

// TestQ217GuardianCalls: a roll under 33 of 100 calls the guardian within
// 100 of where the monster fell on each axis, with the before-battle
// sound; the guardian leaves 200 s later. A roll of 33, or holding the
// guardian's item already, calls none and plays nothing.
func TestQ217GuardianCalls(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name              string
		monster, guardian int32
		hold              int32
		roll              int
		want              bool
	}{
		{"dryad, roll 32", q217Dryad, q217Actea, 0, 32, true},
		{"dryad, roll 33", q217Dryad, q217Actea, 0, 33, false},
		{"dryad, seed held", q217Dryad, q217Actea, q217SeedOfVerdure, 0, false},
		{"dryad, breath held", q217Dryad, q217Actea, q217BreathOfWinds, 0, true},
		{"lirein, roll 32", q217Lirein, q217Luell, 0, 32, true},
		{"lirein, roll 33", q217Lirein, q217Luell, 0, 33, false},
		{"lirein, breath held", q217Lirein, q217Luell, q217BreathOfWinds, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := q217World(t, tc.hold, tc.roll)
			h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(int(tc.monster)), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
			x, y, _ := h.Position()
			killOnPlayerQueue(t, w.srv, w.player, h)
			sent := w.render(t, w.srv.ReadQueued(t, w.srv.Client))
			guardians := npcsOf(w.srv, tc.guardian)
			if !tc.want {
				if len(guardians) != 0 || slices.Contains(sent, q217BeforeBattle) {
					t.Fatalf("%d guardians, sent %q; want none and no sound", len(guardians), sent)
				}
				return
			}
			if len(guardians) != 1 {
				t.Fatalf("%d guardians, want 1", len(guardians))
			}
			if !slices.Contains(sent, q217BeforeBattle) {
				t.Fatalf("sent %q, want the before-battle sound", sent)
			}
			if loc := guardians[0].SpawnLocation(); loc.X < x-100 || loc.X > x+100 || loc.Y < y-100 || loc.Y > y+100 {
				t.Fatalf("the guardian stands at (%d, %d), want within 100 of (%d, %d)", loc.X, loc.Y, x, y)
			}
			// The guardian comes with the dying hook, inside the kill's 3.5 s.
			w.advance(t, 199*time.Second-3500*time.Millisecond)
			if n := len(npcsOf(w.srv, tc.guardian)); n != 1 {
				t.Fatalf("%d guardians 199 s after the kill, want 1", n)
			}
			w.advance(t, 5*time.Second)
			if n := len(npcsOf(w.srv, tc.guardian)); n != 0 {
				t.Fatalf("%d guardians after the despawn, want 0", n)
			}
		})
	}
}
