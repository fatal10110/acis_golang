package quest

import (
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q416"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The Durka Spirit the poison spiders call in Path to an Orc Shaman
// (Q416_PathToAnOrcShaman.onMyDying, Quest.addSpawn): at cond 9 a dying
// Poison Spider or Arachnid Tracker gives a Durka parasite, unless the
// character holds 5 and the draw of 0 to 9 is 0, 6 or 7 and it is 0 or 1, or
// 8 or more: then it takes the parasites and the Durka Spirit stands where the
// spider fell for 120 s. A scenario cannot show a script-spawned NPC appearing
// nor its despawn (#4020 item 5), so this Boot test pins the odds, where the
// spirit stands and the 120 s it stays; the scenarios play the dialogs, the
// items and the fixture monsters' hooks.
const (
	q416Name = "Q416_PathToAnOrcShaman"
	q416Path = "quest.Q416_PathToAnOrcShaman"

	q416Spider   = 20038
	q416Tracker  = 20043
	q416Durka    = 27056
	q416Parasite = 1629
)

// TestQ416SpidersCallTheDurkaSpiritByTheParasitesHeld: the call comes at the
// count and draw the quest names, from either spider, and not at another
// cond.
func TestQ416SpidersCallTheDurkaSpiritByTheParasitesHeld(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		spider int32
		cond   int
		held   int32
		draw   int
		want   int
		called bool
	}{
		{"none held", q416Spider, 9, 0, 0, 1, false},
		{"four held, draw zero", q416Spider, 9, 4, 0, 5, false},
		{"five held, draw zero", q416Spider, 9, 5, 0, 0, true},
		{"five held, draw one", q416Spider, 9, 5, 1, 6, false},
		{"six held, draw one", q416Tracker, 9, 6, 1, 0, true},
		{"six held, draw two", q416Tracker, 9, 6, 2, 7, false},
		{"seven held, draw one", q416Spider, 9, 7, 1, 0, true},
		{"seven held, draw two", q416Spider, 9, 7, 2, 8, false},
		{"eight held, draw nine", q416Tracker, 9, 8, 9, 0, true},
		{"another cond", q416Spider, 10, 8, 0, 8, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			draws := &scriptDraws{}
			draws.queue(tc.draw)
			quest := q416.New()
			w := bootSeamQuest(t, 19, nil, func(srv *gameservertest.Server, objID int32) {
				insertJournal(t, srv.DB,
					journalRow{objID, q416Name, "<state>", val("STARTED")},
					journalRow{objID, q416Name, "<cond>", val(strconv.Itoa(tc.cond))})
				if tc.held > 0 {
					srv.GiveItem(t, objID, q416Parasite, tc.held)
				}
			},
				x1Items(quest.Items),
				gameservertest.WithNPCs(npc.NewTable([]*npc.Template{proofMonster(int(tc.spider)), proofMonster(q416Durka)})),
				gameservertest.WithNpcSpawns(nil),
				gameservertest.WithNPCScripts(
					map[int32]script.NPCKind{tc.spider: script.KindHostile, q416Durka: script.KindHostile},
					[]script.Listing{{Path: q416Path}}, script.Catalog{q416Path: q416.New}),
				gameservertest.WithScriptRand(draws.rand),
			)
			h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(int(tc.spider)), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
			hx, hy, _ := h.Position()
			killOnPlayerQueue(t, w.srv, w.player, h)

			if n := w.srv.PlayerItemCount(t, w.player, q416Parasite); n != tc.want {
				t.Fatalf("Durka parasites held %d times, want %d", n, tc.want)
			}
			called := npcsOf(w.srv, q416Durka)
			if !tc.called {
				if len(called) != 0 {
					t.Fatalf("%d Durka Spirits called, want none", len(called))
				}
				return
			}
			if len(called) != 1 {
				t.Fatalf("%d Durka Spirits called, want 1", len(called))
			}
			if at := called[0].SpawnLocation(); at.X != hx || at.Y != hy {
				t.Fatalf("the Durka Spirit stands at (%d, %d), want where the spider fell (%d, %d)", at.X, at.Y, hx, hy)
			}
			w.srv.Advance(t, 119*time.Second)
			if n := len(npcsOf(w.srv, q416Durka)); n != 1 {
				t.Fatalf("%d Durka Spirits 119 s after the call, want 1", n)
			}
			w.srv.Advance(t, 2*time.Second)
			if n := len(npcsOf(w.srv, q416Durka)); n != 0 {
				t.Fatalf("%d Durka Spirits after the 120 s, want 0", n)
			}
		})
	}
}
