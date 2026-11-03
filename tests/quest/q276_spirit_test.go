package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q276"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Totem of the Hestui's spirit (Q276_TotemOfTheHestui.onMyDying): a Kasha
// bear calls the Kasha bear totem spirit where it fell, and takes every
// parasite held, once the parasites held and a roll of [0, 100) pass one of
// the thresholds; otherwise it gives a parasite. Nothing happens while the
// crystal is held. The scenarios cannot play the call (#4020 item 5: a
// scenario cannot show a script-spawned NPC, and the spawn's random offset
// is not a script draw here while the reference probe pins it as one); they
// place the spirit as an NPC line instead. These Boot tests pin the call.
const (
	q276Name     = "Q276_TotemOfTheHestui"
	q276Path     = "quest.Q276_TotemOfTheHestui"
	q276Bear     = 20479
	q276Spirit   = 27044
	q276Parasite = 1480
	q276Crystal  = 1481
)

// q276World boots a character whose Totem of the Hestui is at cond 1,
// holding parasites parasites and, with crystal, the crystal. The bear's
// roll of [0, 100) draws roll.
func q276World(t *testing.T, parasites int32, crystal bool, roll int) *dialogWorld {
	t.Helper()
	w := bootSeamQuest(t, 15, nil, func(srv *gameservertest.Server, objID int32) {
		insertJournal(t, srv.DB,
			journalRow{objID, q276Name, "<state>", val("STARTED")},
			journalRow{objID, q276Name, "<cond>", val("1")})
		if parasites != 0 {
			srv.GiveItem(t, objID, q276Parasite, parasites)
		}
		if crystal {
			srv.GiveItem(t, objID, q276Crystal, 1)
		}
	},
		x1Items([]int32{q276Parasite, q276Crystal}),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{proofMonster(q276Bear), proofMonster(q276Spirit)})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{q276Bear: script.KindHostile, q276Spirit: script.KindHostile},
			[]script.Listing{{Path: q276Path}},
			script.Catalog{q276Path: q276.New},
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

// TestQ276BearCallsTheSpirit: 79 parasites always call the spirit; 69 with
// a roll up to 20, 59 up to 15, 49 up to 10 and 39 under 2. A call stands
// one spirit within 100 of the bear on each axis and takes every parasite;
// no call gives one parasite more. With the crystal held a bear does
// neither.
func TestQ276BearCallsTheSpirit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		parasites int32
		crystal   bool
		roll      int
		want      bool
	}{
		{"79, roll 99", 79, false, 99, true},
		{"78, roll 21", 78, false, 21, false},
		{"69, roll 20", 69, false, 20, true},
		{"68, roll 20", 68, false, 20, false},
		{"59, roll 15", 59, false, 15, true},
		{"59, roll 16", 59, false, 16, false},
		{"49, roll 10", 49, false, 10, true},
		{"49, roll 11", 49, false, 11, false},
		{"39, roll 1", 39, false, 1, true},
		{"39, roll 2", 39, false, 2, false},
		{"38, roll 0", 38, false, 0, false},
		{"79, crystal held", 79, true, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := q276World(t, tc.parasites, tc.crystal, tc.roll)
			h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(q276Bear), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
			x, y, _ := h.Position()
			killOnPlayerQueue(t, w.srv, w.player, h)
			spirits := npcsOf(w.srv, q276Spirit)
			held := w.srv.PlayerItemCount(t, w.player, q276Parasite)
			switch {
			case tc.crystal:
				if len(spirits) != 0 || held != int(tc.parasites) {
					t.Fatalf("%d spirits, %d parasites; want none and %d", len(spirits), held, tc.parasites)
				}
			case !tc.want:
				if len(spirits) != 0 || held != int(tc.parasites)+1 {
					t.Fatalf("%d spirits, %d parasites; want none and %d", len(spirits), held, tc.parasites+1)
				}
			default:
				if len(spirits) != 1 || held != 0 {
					t.Fatalf("%d spirits, %d parasites; want 1 and none", len(spirits), held)
				}
				if loc := spirits[0].SpawnLocation(); loc.X < x-100 || loc.X > x+100 || loc.Y < y-100 || loc.Y > y+100 {
					t.Fatalf("the spirit stands at (%d, %d), want within 100 of (%d, %d)", loc.X, loc.Y, x, y)
				}
			}
		})
	}
}
