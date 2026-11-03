package quest

import (
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q414"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The Kuruka Ratman Leader the Goblin Tomb Raider Leader calls in Path to an
// Orc Raider (Q414_PathToAnOrcRaider.onMyDying, Quest.addSpawn): at cond 1 a
// dying leader gives a green blood while the character holds no more of it
// than the draw of 0 to 19, and calls the Ratman Leader, taking all the blood,
// once the character holds more. A scenario cannot show a script-spawned NPC
// appearing nor its despawn (#4020 item 5), so this Boot test pins the call,
// where it stands and the 300 s it stays; the scenarios play the dialogs, the
// items and the fixture monsters' hooks.
const (
	q414Name = "Q414_PathToAnOrcRaider"
	q414Path = "quest.Q414_PathToAnOrcRaider"

	q414Raider = 20320
	q414Ratman = 27045
	q414Blood  = 1578
)

// TestQ414RaiderLeaderCallsTheRatmanLeaderOnceTheBloodIsTooMuch: with the
// blood held at most the draw, the dying leader gives one more and calls
// nobody; with more held than the draw, it takes all the blood and the Ratman
// Leader stands where it fell for 300 s. At any cond but 1 nothing happens.
func TestQ414RaiderLeaderCallsTheRatmanLeaderOnceTheBloodIsTooMuch(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		cond     int
		blood    int32
		draw     int
		wantBlod int
		called   bool
	}{
		{"none held", 1, 0, 0, 1, false},
		{"held equals the draw", 1, 5, 5, 6, false},
		{"held is more than the draw", 1, 5, 4, 0, true},
		{"one held, draw zero", 1, 1, 0, 0, true},
		{"another cond", 2, 5, 4, 5, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			draws := &scriptDraws{}
			draws.queue(tc.draw)
			quest := q414.New()
			w := bootSeamQuest(t, 19, nil, func(srv *gameservertest.Server, objID int32) {
				insertJournal(t, srv.DB,
					journalRow{objID, q414Name, "<state>", val("STARTED")},
					journalRow{objID, q414Name, "<cond>", val(strconv.Itoa(tc.cond))})
				if tc.blood > 0 {
					srv.GiveItem(t, objID, q414Blood, tc.blood)
				}
			},
				x1Items(quest.Items),
				gameservertest.WithNPCs(npc.NewTable([]*npc.Template{proofMonster(q414Raider), proofMonster(q414Ratman)})),
				gameservertest.WithNpcSpawns(nil),
				gameservertest.WithNPCScripts(
					map[int32]script.NPCKind{q414Raider: script.KindHostile, q414Ratman: script.KindHostile},
					[]script.Listing{{Path: q414Path}}, script.Catalog{q414Path: q414.New}),
				gameservertest.WithScriptRand(draws.rand),
			)
			h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(q414Raider), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
			hx, hy, _ := h.Position()
			killOnPlayerQueue(t, w.srv, w.player, h)

			if n := w.srv.PlayerItemCount(t, w.player, q414Blood); n != tc.wantBlod {
				t.Fatalf("green blood held %d times, want %d", n, tc.wantBlod)
			}
			called := npcsOf(w.srv, q414Ratman)
			if !tc.called {
				if len(called) != 0 {
					t.Fatalf("%d Ratman Leaders called, want none", len(called))
				}
				return
			}
			if len(called) != 1 {
				t.Fatalf("%d Ratman Leaders called, want 1", len(called))
			}
			if at := called[0].SpawnLocation(); at.X != hx || at.Y != hy {
				t.Fatalf("the Ratman Leader stands at (%d, %d), want where the raider fell (%d, %d)", at.X, at.Y, hx, hy)
			}
			w.srv.Advance(t, 299*time.Second)
			if n := len(npcsOf(w.srv, q414Ratman)); n != 1 {
				t.Fatalf("%d Ratman Leaders 299 s after the call, want 1", n)
			}
			w.srv.Advance(t, 2*time.Second)
			if n := len(npcsOf(w.srv, q414Ratman)); n != 0 {
				t.Fatalf("%d Ratman Leaders after the 300 s, want 0", n)
			}
		})
	}
}
