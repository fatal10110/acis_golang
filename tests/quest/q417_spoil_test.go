package quest

import (
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q417"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The kill of Path to Become a Scavenger (Q417_PathToBecomeAScavenger.onMyDying)
// that a scenario cannot show: the honey bear a hunter bear calls stands where
// the hunter fell (a scenario asserts no place of a spawned NPC). This Boot
// test pins the draw of the hunter bear's counter and the honey bear's place
// and life; the scenarios play the dialogs, the items, the spoiled honey
// jars and beads (q417_*_spoiled*.scenario) and the called honey bear's count.
const (
	q417Name = "Q417_PathToBecomeAScavenger"
	q417Path = "quest.Q417_PathToBecomeAScavenger"

	q417HunterBear = 20777
	q417HoneyBear  = 27058
)

// q417Kill boots a character with the journal rows, holding the items of held, spawns monster beside
// it, answers draws in order and has the character kill it.
func q417Kill(t *testing.T, monster int32, journal []journalRow, held map[int32]int32, draws ...int) (*dialogWorld, *npc.Hostile) {
	t.Helper()
	d := &scriptDraws{}
	d.queue(draws...)
	quest := q417.New()
	templates := []*npc.Template{proofMonster(int(monster))}
	if monster != q417HoneyBear {
		templates = append(templates, proofMonster(q417HoneyBear))
	}
	w := bootSeamQuest(t, 19, nil, func(srv *gameservertest.Server, objID int32) {
		rows := []journalRow{{objID, q417Name, "<state>", val("STARTED")}}
		for _, r := range journal {
			r.charID = objID
			rows = append(rows, r)
		}
		insertJournal(t, srv.DB, rows...)
		for id, n := range held {
			if n > 0 {
				srv.GiveItem(t, objID, id, n)
			}
		}
	},
		x1Items(quest.Items),
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{monster: script.KindHostile, q417HoneyBear: script.KindHostile},
			[]script.Listing{{Path: q417Path}}, script.Catalog{q417Path: q417.New}),
		gameservertest.WithScriptRand(d.rand),
	)
	h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(int(monster)), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
	killOnPlayerQueue(t, w.srv, w.player, h)
	return w, h
}

func q417Cond(n int) journalRow { return journalRow{0, q417Name, "<cond>", val(strconv.Itoa(n))} }

// TestQ417HunterBearCountsThenCallsTheHoneyBear: a hunter bear at cond 5
// counts one in "step" up to 20; past it the draw of 0 to 99 against
// (step - 20) * 10 calls the honey bear where the hunter fell for 300 s and
// forgets the count, else the count goes on.
func TestQ417HunterBearCountsThenCallsTheHoneyBear(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		cond     int
		step     string
		draw     int
		wantStep string
		called   bool
	}{
		{"first kill", 5, "", 0, "1", false},
		{"at twenty", 5, "20", 0, "21", false},
		{"past twenty, draw over", 5, "21", 11, "22", false},
		{"past twenty, draw at the odds", 5, "21", 10, "", true},
		{"far past twenty", 5, "30", 99, "", true},
		{"another cond", 6, "21", 0, "21", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			journal := []journalRow{q417Cond(tc.cond)}
			if tc.step != "" {
				journal = append(journal, journalRow{0, q417Name, "step", val(tc.step)})
			}
			w, h := q417Kill(t, q417HunterBear, journal, nil, tc.draw)
			vars := w.srv.QuestVars(t, w.player, q417Name)
			if got := vars["step"]; got != tc.wantStep {
				t.Fatalf("step = %q, want %q", got, tc.wantStep)
			}
			called := npcsOf(w.srv, q417HoneyBear)
			if !tc.called {
				if len(called) != 0 {
					t.Fatalf("%d honey bears called, want none", len(called))
				}
				return
			}
			if len(called) != 1 {
				t.Fatalf("%d honey bears called, want 1", len(called))
			}
			hx, hy, _ := h.Position()
			if at := called[0].SpawnLocation(); at.X != hx || at.Y != hy {
				t.Fatalf("the honey bear stands at (%d, %d), want where the hunter fell (%d, %d)", at.X, at.Y, hx, hy)
			}
			w.srv.Advance(t, 299*time.Second)
			if n := len(npcsOf(w.srv, q417HoneyBear)); n != 1 {
				t.Fatalf("%d honey bears 299 s after the call, want 1", n)
			}
			w.srv.Advance(t, 2*time.Second)
			if n := len(npcsOf(w.srv, q417HoneyBear)); n != 0 {
				t.Fatalf("%d honey bears after the 300 s, want 0", n)
			}
		})
	}
}
