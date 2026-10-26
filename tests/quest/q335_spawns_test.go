package quest

import (
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q335"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The monsters Song of the Hunter calls out as others die
// (Q335_SongOfTheHunter onMyDying): the Breka overlords, Windsus Aleph and
// the Tarlk raiders of Grey's tests, and the monsters of Tor's requests. A
// scenario cannot show a script-spawned NPC appearing, speaking or hating
// the player, nor act on one (#4020 item 5), so these Boot tests pin them;
// the scenarios play the dialogs, the drops and Cybellin's crystals.
const (
	q335Name  = "Q335_SongOfTheHunter"
	q335Path  = "quest.Q335_SongOfTheHunter"
	q335Grey  = 30744
	q335Tor   = 30745
	q335Cybel = 30746

	q335BrekaWarrior = 20271
	q335Windsus      = 20553
	q335Tarlk        = 20571
	q335LetoOverlord = 20582
	q335HatarBoss    = 20662
	q335GokMagok     = 27160
	q335Stormtrooper = 27150
	q335Gremlin      = 27149
	q335LetoNarak    = 27157
	q335Aleph        = 27143

	q335HakaHead  = 3711
	q335AlephSkin = 3714
	q335AthuHead  = 3722
)

// q335World boots a character of level whose quest is STARTED at cond with
// the quest variables rows, holding holds. The script random answers draws.
func q335World(t *testing.T, draws *scriptDraws, level int, cond int, rows map[string]string, holds map[int32]int32) *dialogWorld {
	t.Helper()
	quest := q335.New()
	templates := []*npc.Template{
		gameservertest.FolkTemplate("Folk", q335Grey), gameservertest.FolkTemplate("Folk", q335Tor), gameservertest.FolkTemplate("Folk", q335Cybel),
	}
	kinds := map[int32]script.NPCKind{q335Grey: script.KindFolk, q335Tor: script.KindFolk, q335Cybel: script.KindFolk}
	monsters := append(append([]int32{}, quest.Bind[script.EventMyDying]...), q335Stormtrooper)
	for _, id := range monsters {
		if _, ok := kinds[id]; ok {
			continue
		}
		templates = append(templates, proofMonster(int(id)))
		kinds[id] = script.KindHostile
	}
	return bootSeamQuest(t, level, nil, func(srv *gameservertest.Server, objID int32) {
		insertJournal(t, srv.DB,
			journalRow{objID, q335Name, "<state>", val("STARTED")},
			journalRow{objID, q335Name, "<cond>", val(strconv.Itoa(cond))})
		for k, v := range rows {
			insertJournal(t, srv.DB, journalRow{objID, q335Name, k, val(v)})
		}
		for id, count := range holds {
			srv.GiveItem(t, objID, id, count)
		}
	},
		x1Items(quest.Items),
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: q335Path}}, script.Catalog{q335Path: q335.New}),
		gameservertest.WithScriptRand(draws.rand),
	)
}

// q335Kill kills a fresh monster of npc id beside the character and returns
// what the character was sent meanwhile.
func (w *dialogWorld) q335Kill(t *testing.T, id int32) []string {
	t.Helper()
	h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(int(id)), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
	killOnPlayerQueue(t, w.srv, w.player, h)
	return w.render(t, w.srv.ReadQueued(t, w.srv.Client))
}

// q335Called checks that exactly count NPCs of id stand within 100 of the
// monster q335Kill killed, each set on the character with the most hate when
// hates and on nobody otherwise; it returns them.
func q335Called(t *testing.T, w *dialogWorld, id int32, count int, hates bool) []*script.NPC {
	t.Helper()
	called := npcsOf(w.srv, id)
	if len(called) != count {
		t.Fatalf("%d NPCs %d called, want %d", len(called), id, count)
	}
	for _, n := range called {
		if loc := n.SpawnLocation(); loc.X < w.at.X+40-100 || loc.X > w.at.X+40+100 || loc.Y < w.at.Y-100 || loc.Y > w.at.Y+100 {
			t.Fatalf("NPC %d stands at %+v, want within 100 of the killed monster at %+v", id, loc, w.at)
		}
		h := n.MostHated()
		if hates && (h == nil || h.ObjectID() != w.player) {
			t.Fatalf("NPC %d hates %v most, want the player", id, h)
		}
		if !hates && h != nil {
			t.Fatalf("NPC %d hates %v, want nobody", id, h)
		}
	}
	return called
}

// q335Gone checks that count NPCs of id are there 290 s after the call and
// none is left 15 s later.
func q335Gone(t *testing.T, w *dialogWorld, id int32, count int) {
	t.Helper()
	w.advance(t, 290*time.Second)
	if n := len(npcsOf(w.srv, id)); n != count {
		t.Fatalf("%d NPCs %d 290 s after the call, want %d", n, id, count)
	}
	w.advance(t, 15*time.Second)
	if n := len(npcsOf(w.srv, id)); n != 0 {
		t.Fatalf("%d NPCs %d after the call is over, want 0", n, id)
	}
}

// q335Rows asserts the quest variable key.
func q335Row(t *testing.T, w *dialogWorld, key, want string) {
	t.Helper()
	if got := questRows(t, w.srv, w.player, q335Name)[key]; got != want {
		t.Fatalf("quest variable %s = %q, want %q", key, got, want)
	}
}

// TestQ335BrekaWarriorCallsTheOverlordsInOrder: at the first test, a Breka
// orc warrior killed on a draw under 20 of 100 calls the first overlord whose
// head is not held, Haka, Jaka, then Marka, and none once the three heads are
// held; a draw of 20 calls none. They are gone 300 s after.
func TestQ335BrekaWarriorCallsTheOverlordsInOrder(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		holds map[int32]int32
		draw  int
		want  int32
	}{
		{"no head", nil, 19, 27140},
		{"Haka's head", map[int32]int32{3711: 1}, 19, 27141},
		{"Haka's and Jaka's heads", map[int32]int32{3711: 1, 3712: 1}, 19, 27142},
		{"all heads", map[int32]int32{3711: 1, 3712: 1, 3713: 1}, 19, 0},
		{"a draw of 20", nil, 20, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			draws := &scriptDraws{}
			w := q335World(t, draws, 35, 1, nil, tc.holds)
			draws.queue(tc.draw)
			w.q335Kill(t, q335BrekaWarrior)
			for _, id := range []int32{27140, 27141, 27142} {
				if id == tc.want {
					q335Called(t, w, id, 1, false)
					q335Gone(t, w, id, 1)
					continue
				}
				if n := len(npcsOf(w.srv, id)); n != 0 {
					t.Fatalf("%d NPCs %d called, want 0", n, id)
				}
			}
		})
	}
}

// TestQ335WindsusCallsAleph: at the first test, a Windsus killed on a draw
// under 20 of 100 calls Windsus Aleph while no Aleph skin is held; with the
// skin it draws nothing at all.
func TestQ335WindsusCallsAleph(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		holds map[int32]int32
		draw  int
		want  int
		draws int
	}{
		{"no skin", nil, 19, 1, 1},
		{"a draw of 20", nil, 20, 0, 1},
		{"the skin held", map[int32]int32{q335AlephSkin: 1}, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			draws := &scriptDraws{}
			w := q335World(t, draws, 35, 1, nil, tc.holds)
			draws.queue(tc.draw)
			w.q335Kill(t, q335Windsus)
			q335Called(t, w, q335Aleph, tc.want, false)
			if tc.want > 0 {
				q335Gone(t, w, q335Aleph, tc.want)
			}
			if len(draws.bounds) != tc.draws {
				t.Fatalf("draws %v, want %d", draws.bounds, tc.draws)
			}
		})
	}
}

// TestQ335TarlkWarriorCallsTheRaidersInOrder: while Grey's second test runs,
// a Tarlk bugbear warrior killed on a draw under 20 of 100 calls the first
// raider whose head is not held, Athu to Kalath, and none once the five heads
// are held; with no test running it draws nothing.
func TestQ335TarlkWarriorCallsTheRaidersInOrder(t *testing.T) {
	t.Parallel()
	for held := range 6 {
		t.Run("heads held "+strconv.Itoa(held), func(t *testing.T) {
			t.Parallel()
			holds := map[int32]int32{}
			for i := range held {
				holds[q335AthuHead+int32(i)] = 1
			}
			draws := &scriptDraws{}
			w := q335World(t, draws, 45, 2, map[string]string{"grey2": "1"}, holds)
			draws.queue(19)
			w.q335Kill(t, q335Tarlk)
			for i := range 5 {
				id := int32(27144 + i)
				if i == held {
					q335Called(t, w, id, 1, false)
					q335Gone(t, w, id, 1)
					continue
				}
				if n := len(npcsOf(w.srv, id)); n != 0 {
					t.Fatalf("%d raiders %d called, want 0", n, id)
				}
			}
		})
	}
	t.Run("no test running", func(t *testing.T) {
		t.Parallel()
		draws := &scriptDraws{}
		w := q335World(t, draws, 45, 2, nil, nil)
		w.q335Kill(t, q335Tarlk)
		if len(draws.bounds) != 0 || len(npcsOf(w.srv, 27144)) != 0 {
			t.Fatalf("draws %v, raiders %d, want none", draws.bounds, len(npcsOf(w.srv, 27144)))
		}
	})
}

// TestQ335TorRequestsCallMonsters: while a request of Tor runs, its monsters
// killed call another on a draw under their chance, set on the character,
// the callers of a reached request only once the item is held in full, the
// others only while it is not.
func TestQ335TorRequestsCallMonsters(t *testing.T) {
	t.Parallel()
	t.Run("a chief called by his guard", func(t *testing.T) {
		t.Parallel()
		draws := &scriptDraws{}
		w := q335World(t, draws, 45, 2, map[string]string{"tor": "3739"}, map[int32]int32{3739: 1})
		draws.queue(19)
		w.q335Kill(t, q335LetoOverlord)
		q335Called(t, w, q335LetoNarak, 1, true)
		q335Gone(t, w, q335LetoNarak, 1)
	})
	t.Run("a draw of 20", func(t *testing.T) {
		t.Parallel()
		draws := &scriptDraws{}
		w := q335World(t, draws, 45, 2, map[string]string{"tor": "3739"}, map[int32]int32{3739: 1})
		draws.queue(20)
		w.q335Kill(t, q335LetoOverlord)
		q335Called(t, w, q335LetoNarak, 0, true)
	})
	t.Run("the fang held in full", func(t *testing.T) {
		t.Parallel()
		draws := &scriptDraws{}
		w := q335World(t, draws, 45, 2, map[string]string{"tor": "3739"}, map[int32]int32{3739: 1, 3781: 1})
		draws.queue(0)
		w.q335Kill(t, q335LetoOverlord)
		q335Called(t, w, q335LetoNarak, 0, true)
	})
	t.Run("another request", func(t *testing.T) {
		t.Parallel()
		draws := &scriptDraws{}
		w := q335World(t, draws, 45, 2, map[string]string{"tor": "3727"}, map[int32]int32{3727: 1})
		draws.queue(0)
		w.q335Kill(t, q335LetoOverlord)
		q335Called(t, w, q335LetoNarak, 0, true)
	})
	t.Run("two troopers called by the chief once the tablet is held", func(t *testing.T) {
		t.Parallel()
		draws := &scriptDraws{}
		w := q335World(t, draws, 50, 2, map[string]string{"tor": "3746"}, map[int32]int32{3746: 1})
		// The drop of the tablet, then the draw of the call.
		draws.queue(0, 49)
		lines := w.q335Kill(t, q335GokMagok)
		if n := w.srv.PlayerItemCount(t, w.player, 3788); n != 1 {
			t.Fatalf("%d tablets, want 1", n)
		}
		for _, n := range q335Called(t, w, q335Stormtrooper, 2, true) {
			wantLines(t, "the call", lines, q334Say(n, 33511))
		}
		q335Gone(t, w, q335Stormtrooper, 2)
	})
	t.Run("no trooper on a draw of 50", func(t *testing.T) {
		t.Parallel()
		draws := &scriptDraws{}
		w := q335World(t, draws, 50, 2, map[string]string{"tor": "3746"}, map[int32]int32{3746: 1})
		// The drop of the tablet, then the draw of the call.
		draws.queue(0, 50)
		w.q335Kill(t, q335GokMagok)
		q335Called(t, w, q335Stormtrooper, 0, true)
	})
	t.Run("Gok Magok called by a Grandis while the tablet is not held", func(t *testing.T) {
		t.Parallel()
		draws := &scriptDraws{}
		w := q335World(t, draws, 50, 2, map[string]string{"tor": "3746"}, map[int32]int32{3746: 1})
		draws.queue(19)
		w.q335Kill(t, 20554)
		q335Called(t, w, q335GokMagok, 1, true)
		q335Gone(t, w, q335GokMagok, 1)
	})
	t.Run("no Gok Magok once the tablet is held", func(t *testing.T) {
		t.Parallel()
		draws := &scriptDraws{}
		w := q335World(t, draws, 50, 2, map[string]string{"tor": "3746"}, map[int32]int32{3746: 1, 3788: 1})
		draws.queue(0)
		w.q335Kill(t, 20554)
		q335Called(t, w, q335GokMagok, 0, true)
	})
	t.Run("a gremlin called by a ratman", func(t *testing.T) {
		t.Parallel()
		draws := &scriptDraws{}
		w := q335World(t, draws, 50, 3, map[string]string{"tor": "3752"}, map[int32]int32{3752: 1})
		// The drop of a coin, then the draw of the call.
		draws.queue(0, 9)
		lines := w.q335Kill(t, q335HatarBoss)
		if n := w.srv.PlayerItemCount(t, w.player, 3794); n != 1 {
			t.Fatalf("%d coins, want 1", n)
		}
		for _, n := range q335Called(t, w, q335Gremlin, 1, true) {
			wantLines(t, "the call", lines, q334Say(n, 33512))
		}
		q335Gone(t, w, q335Gremlin, 1)
	})
	t.Run("no gremlin once the coins are held in full", func(t *testing.T) {
		t.Parallel()
		draws := &scriptDraws{}
		w := q335World(t, draws, 50, 3, map[string]string{"tor": "3752"}, map[int32]int32{3752: 1, 3794: 20})
		draws.queue(0, 0)
		w.q335Kill(t, q335HatarBoss)
		q335Called(t, w, q335Gremlin, 0, true)
		q335Row(t, w, "tor", "3752")
	})
}
