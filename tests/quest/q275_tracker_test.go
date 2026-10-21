package quest

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q275"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Dark Winged Spies' tracker (Q275_DarkWingedSpies.onMyDying): a Darkwing
// bat that does not bring the seventieth fang calls a Varangka's tracker
// where it fell, and gives a Varangka parasite, on a roll under 10 of 100
// while the fangs held are more than 10 and fewer than 66. The scenarios
// cannot play the call (#4020 item 5: a scenario cannot show a
// script-spawned NPC, and the spawn's random offset is not a script draw
// here while the reference probe pins it as one); they place the tracker as
// an NPC line instead. These Boot tests pin the call.
const (
	q275Name     = "Q275_DarkWingedSpies"
	q275Path     = "quest.Q275_DarkWingedSpies"
	q275Bat      = 20316
	q275Tracker  = 27043
	q275Fang     = 1478
	q275Parasite = 1479
)

// q275World boots a character whose Dark Winged Spies is at cond 1,
// holding fangs fangs. The tracker's roll of [0, 100) draws roll.
func q275World(t *testing.T, fangs int32, roll int) *dialogWorld {
	t.Helper()
	w := bootSeamQuest(t, 11, nil, func(srv *gameservertest.Server, objID int32) {
		insertJournal(t, srv.DB,
			journalRow{objID, q275Name, "<state>", val("STARTED")},
			journalRow{objID, q275Name, "<cond>", val("1")})
		if fangs != 0 {
			srv.GiveItem(t, objID, q275Fang, fangs)
		}
	},
		x1Items([]int32{q275Fang, q275Parasite}),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{proofMonster(q275Bat), proofMonster(q275Tracker)})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{q275Bat: script.KindHostile, q275Tracker: script.KindHostile},
			[]script.Listing{{Path: q275Path}},
			script.Catalog{q275Path: q275.New},
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

// TestQ275BatCallsTheTracker: the bat's fang comes first, so the fangs
// counted are the ones held after it. A roll of 9 with 11 to 65 fangs calls
// one tracker within 100 of the bat on each axis and gives one parasite; the
// tracker stays (no despawn). A roll of 10, or 10 or 66 fangs, calls none.
func TestQ275BatCallsTheTracker(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		fangs int32 // held before the kill
		roll  int
		want  bool
	}{
		{"11 fangs, roll 9", 10, 9, true},
		{"65 fangs, roll 9", 64, 9, true},
		{"11 fangs, roll 10", 10, 10, false},
		{"10 fangs, roll 0", 9, 0, false},
		{"66 fangs, roll 0", 65, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := q275World(t, tc.fangs, tc.roll)
			h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(q275Bat), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
			x, y, _ := h.Position()
			killOnPlayerQueue(t, w.srv, w.player, h)
			if n := w.srv.PlayerItemCount(t, w.player, q275Fang); n != int(tc.fangs)+1 {
				t.Fatalf("%d fangs, want %d", n, tc.fangs+1)
			}
			trackers := npcsOf(w.srv, q275Tracker)
			parasites := w.srv.PlayerItemCount(t, w.player, q275Parasite)
			if !tc.want {
				if len(trackers) != 0 || parasites != 0 {
					t.Fatalf("%d trackers, %d parasites; want none", len(trackers), parasites)
				}
				return
			}
			if len(trackers) != 1 || parasites != 1 {
				t.Fatalf("%d trackers, %d parasites; want 1 and 1", len(trackers), parasites)
			}
			if loc := trackers[0].SpawnLocation(); loc.X < x-100 || loc.X > x+100 || loc.Y < y-100 || loc.Y > y+100 {
				t.Fatalf("the tracker stands at (%d, %d), want within 100 of (%d, %d)", loc.X, loc.Y, x, y)
			}
			w.advance(t, 30*time.Minute)
			if n := len(npcsOf(w.srv, q275Tracker)); n != 1 {
				t.Fatalf("%d trackers 30 minutes later, want 1", n)
			}
		})
	}
}
