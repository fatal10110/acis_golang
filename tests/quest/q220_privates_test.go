package quest

import (
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q220"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Testimony of Glory's script-spawned NPCs (Q220_TestimonyOfGlory): the
// chiefs' privates, and the Revenant of Tantos a Ragna orc leaves behind,
// with the lines they speak. A scenario cannot show a script-spawned NPC
// appearing, nor act on one, nor an NPC's chat line (#4020), so these Boot
// tests pin them; the scenarios play the rest of the quest, and the kills
// of the chiefs' privates as monsters of an NPC line.
const (
	q220Name = "Q220_TestimonyOfGlory"
	q220Path = "quest.Q220_TestimonyOfGlory"

	q220Voltar = 30615
	q220Kepra  = 30616
	q220Burai  = 30617

	q220Pashika  = 27080
	q220Vultus   = 27081
	q220Enku     = 27082
	q220Makum    = 27083
	q220Revenant = 27086
	q220Overlord = 20778
	q220Seer     = 20779

	q220ChiantaOrder1  = 3210
	q220ManakiaLetter1 = 3228
	q220ManakiaLetter2 = 3229
	q220KasmanLetter2  = 3231
	q220TanapiOrder    = 3235
	q220ScepterTantos  = 3236
)

// q220World boots a level 38 character whose Testimony of Glory is at cond,
// holding hold, over the quest's chiefs and monsters.
func q220World(t *testing.T, cond int, hold ...int32) *dialogWorld {
	t.Helper()
	pages := chatPages(questPages(q220Name, map[string][]string{
		"30615-02.htm": {"30615-04.htm"}, "30615-04.htm": nil, "30615-05.htm": nil,
		"30616-02.htm": {"30616-05.htm"}, "30616-05.htm": nil,
		"30617-02.htm": {"30617-04.htm"}, "30617-04.htm": nil,
	}), q220Voltar, q220Kepra, q220Burai)
	var items []int32
	for id := int32(3203); id <= 3237; id++ {
		items = append(items, id)
	}
	talkers := []int32{30501, 30514, 30515, 30565, 30571, q220Voltar, q220Kepra, q220Burai, 30618, 30619, 30642}
	dying := []int32{
		20192, 20234, 20550, 20563, 20583, 20584, 20585, 20586, 20587, 20588, 20601, 20602,
		q220Overlord, q220Seer, q220Pashika, q220Vultus, q220Enku, q220Makum, q220Revenant,
	}
	kinds := map[int32]script.NPCKind{}
	templates := []*npc.Template{}
	for _, id := range talkers {
		kinds[id] = script.KindFolk
		templates = append(templates, gameservertest.FolkTemplate("Folk", int(id)))
	}
	for _, id := range dying {
		kinds[id] = script.KindHostile
		templates = append(templates, proofMonster(int(id)))
	}
	w := bootSeamQuest(t, 38, pages, func(srv *gameservertest.Server, objID int32) {
		insertJournal(t, srv.DB,
			journalRow{objID, q220Name, "<state>", val("STARTED")},
			journalRow{objID, q220Name, "<cond>", val(strconv.Itoa(cond))})
		for _, id := range hold {
			srv.GiveItem(t, objID, id, 1)
		}
	},
		x1Items(items),
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: q220Path}}, script.Catalog{q220Path: q220.New}),
	)
	w.roles[w.player] = "player"
	return w
}

// q220Event talks to the NPC role, expecting its page, and sends the quest's
// bypass event.
func (w *dialogWorld) q220Event(t *testing.T, role, page, event string) []string {
	t.Helper()
	if got := w.talk(t, role); !shows(got, page) {
		t.Fatalf("%s: got %q, want %s", role, got, page)
	}
	return w.send(t, encodeBypass("Quest "+q220Name+" "+event))
}

// TestQ220ChiefsCallTheirPrivates: the event that hands a chief's glove over
// calls the chief's privates, and so does the chief's dialog while the glove
// is held; both only while the chief has no private left. The privates leave
// 200 s after the call, and the next dialog calls them again.
func TestQ220ChiefsCallTheirPrivates(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                         string
		chief                        int
		letter                       int32
		letterPage, event, glovePage string
		privates                     map[int32]int
	}{
		{
			"Voltar", q220Voltar, q220ManakiaLetter1, "30615-02.htm", "30615-04.htm", "30615-05.htm",
			map[int32]int{q220Pashika: 1, q220Vultus: 1},
		},
		{
			"Kepra", q220Kepra, q220ManakiaLetter2, "30616-02.htm", "30616-05.htm", "30616-05.htm",
			map[int32]int{q220Enku: 4},
		},
		{
			"Burai", q220Burai, q220KasmanLetter2, "30617-02.htm", "30617-04.htm", "30617-04.htm",
			map[int32]int{q220Makum: 2},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := q220World(t, 4, tc.letter)
			w.spawnFolk(t, "chief", tc.chief, 40, 0, 0)
			chief := npcsOf(w.srv, int32(tc.chief))[0]
			count := func(want int, when string) {
				t.Helper()
				for id, n := range tc.privates {
					if got := len(npcsOf(w.srv, id)); got != n*want {
						t.Fatalf("%s: %d NPCs %d, want %d", when, got, id, n*want)
					}
				}
			}

			count(0, "before the event")
			if got := w.q220Event(t, "chief", tc.letterPage, tc.event); !shows(got, tc.event) {
				t.Fatalf("the event: got %q, want %s", got, tc.event)
			}
			count(1, "after the event")
			for id := range tc.privates {
				for _, p := range npcsOf(w.srv, id) {
					if m := p.Master(); m == nil || m.ObjectID() != chief.ObjectID() {
						t.Fatalf("a private %d has master %v, want the chief %d", id, m, chief.ObjectID())
					}
				}
			}

			// The glove is held and the privates stand: the dialog calls no more.
			if got := w.talk(t, "chief"); !shows(got, tc.glovePage) {
				t.Fatalf("the dialog: got %q, want %s", got, tc.glovePage)
			}
			count(1, "after the dialog")

			w.advance(t, 199*time.Second)
			count(1, "199 s after the call")
			w.advance(t, 2*time.Second)
			count(0, "after the despawn")

			if got := w.talk(t, "chief"); !shows(got, tc.glovePage) {
				t.Fatalf("the dialog after the despawn: got %q, want %s", got, tc.glovePage)
			}
			count(1, "after the dialog that follows the despawn")
		})
	}
}

// q220Hostile places a hostile NPC of npcID dx east of the player.
func q220Hostile(w *dialogWorld, t *testing.T, role string, npcID int, dx int) *npc.Hostile {
	t.Helper()
	h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(npcID),
		location.Location{X: w.at.X + dx, Y: w.at.Y, Z: w.at.Z})
	w.roles[h.ObjectID()], w.npcs[role] = role, h.ObjectID()
	w.srv.ReadQueued(t, w.srv.Client)
	return h
}

// q220Says renders what the player was sent since the last read and keeps
// the NPC lines.
func (w *dialogWorld) q220Says(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, l := range w.render(t, w.srv.ReadQueued(t, w.srv.Client)) {
		if len(l) > 9 && l[:9] == "S NpcSay " {
			out = append(out, l)
		}
	}
	return out
}

// TestQ220RagnaOrcsWakeTheRevenant: at cond 9 the first hit on a Ragna orc
// makes it speak, once; its death, three seconds on, makes it speak again
// and leaves the Revenant of Tantos where it fell, for 200 s. At another
// cond the orc says nothing and leaves nothing.
func TestQ220RagnaOrcsWakeTheRevenant(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		id   int
		cond int
	}{
		{"overlord", q220Overlord, 9},
		{"seer", q220Seer, 9},
		{"overlord, cond 8", q220Overlord, 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := q220World(t, tc.cond, q220TanapiOrder)
			ragna := q220Hostile(w, t, "ragna", tc.id, 40)
			at := script.LocOf(script.NPCOf(ragna))
			speaks := tc.cond == 9

			hit(t, w.srv, w.player, ragna, 1)
			got := w.q220Says(t)
			if speaks {
				if want := say("ragna", int32(tc.id), 22051); len(got) != 1 || got[0] != want {
					t.Fatalf("the first hit: %q, want %q", got, want)
				}
			} else if len(got) != 0 {
				t.Fatalf("the first hit: %q, want no line", got)
			}
			hit(t, w.srv, w.player, ragna, 1)
			if got := w.q220Says(t); len(got) != 0 {
				t.Fatalf("the second hit: %q, want no line", got)
			}

			killOnPlayerQueue(t, w.srv, w.player, ragna)
			got = w.q220Says(t)
			revenants := npcsOf(w.srv, q220Revenant)
			if !speaks {
				if len(got) != 0 || len(revenants) != 0 {
					t.Fatalf("the death: %q and %d Revenants, want none", got, len(revenants))
				}
				return
			}
			if want := say("ragna", int32(tc.id), 22052); len(got) != 1 || got[0] != want {
				t.Fatalf("the death: %q, want %q", got, want)
			}
			if len(revenants) != 1 {
				t.Fatalf("%d Revenants after the death, want 1", len(revenants))
			}
			if loc := revenants[0].SpawnLocation(); loc.X != at.X || loc.Y != at.Y || abs(loc.Z-at.Z) > 32 || loc.Heading != 0 {
				t.Fatalf("the Revenant stands at %+v, want (%d, %d, %d) heading 0", loc, at.X, at.Y, at.Z)
			}
			w.advance(t, 199*time.Second)
			if n := len(npcsOf(w.srv, q220Revenant)); n != 1 {
				t.Fatalf("%d Revenants 199 s after the call, want 1", n)
			}
			w.advance(t, 2*time.Second)
			if n := len(npcsOf(w.srv, q220Revenant)); n != 0 {
				t.Fatalf("%d Revenants after the despawn, want 0", n)
			}
		})
	}
}

// TestQ220RevenantSpeaksAndGivesTheScepter: at cond 9 the first hit on the
// Revenant of Tantos makes it speak; a hit that leaves it under a third of
// its HP makes it speak once more; each line once. Its death, three seconds
// on, gives the scepter, sets cond 10 and makes it speak a last time.
func TestQ220RevenantSpeaksAndGivesTheScepter(t *testing.T) {
	t.Parallel()
	w := q220World(t, 9, q220TanapiOrder)
	rev := q220Hostile(w, t, "revenant", q220Revenant, 40)

	hit(t, w.srv, w.player, rev, 10)
	if got, want := w.q220Says(t), say("revenant", q220Revenant, 22055); len(got) != 1 || got[0] != want {
		t.Fatalf("the first hit: %q, want %q", got, want)
	}
	// The hook runs before the hit lands: the HP it reads are those the hit found.
	// The next hit leaves it at three tenths of its HP.
	hit(t, w.srv, w.player, rev, rev.CurrentHP()-rev.MaxHP()*3/10)
	if got := w.q220Says(t); len(got) != 0 {
		t.Fatalf("the hit that takes it under a third: %q, want no line", got)
	}
	hit(t, w.srv, w.player, rev, 1)
	if got, want := w.q220Says(t), say("revenant", q220Revenant, 22057); len(got) != 1 || got[0] != want {
		t.Fatalf("the hit on it under a third: %q, want %q", got, want)
	}
	hit(t, w.srv, w.player, rev, 1)
	if got := w.q220Says(t); len(got) != 0 {
		t.Fatalf("the next hit: %q, want no line", got)
	}

	killOnPlayerQueue(t, w.srv, w.player, rev)
	if got, want := w.q220Says(t), say("revenant", q220Revenant, 22056); len(got) != 1 || got[0] != want {
		t.Fatalf("the death: %q, want %q", got, want)
	}
	if rows := questRows(t, w.srv, w.player, q220Name); rows["<cond>"] != "10" {
		t.Fatalf("rows %v after the death, want cond 10", rows)
	}
	if n := w.srv.PlayerItemCount(t, w.player, q220ScepterTantos); n != 1 {
		t.Fatalf("%d scepters of Tantos, want 1", n)
	}
}

// TestQ220RevenantLeavesNothingBeforeCond9: a Revenant hit and killed at
// another cond says nothing and gives nothing.
func TestQ220RevenantLeavesNothingBeforeCond9(t *testing.T) {
	t.Parallel()
	w := q220World(t, 8, q220TanapiOrder)
	rev := q220Hostile(w, t, "revenant", q220Revenant, 40)

	hit(t, w.srv, w.player, rev, rev.MaxHP()*9/10)
	killOnPlayerQueue(t, w.srv, w.player, rev)
	if got := w.q220Says(t); len(got) != 0 {
		t.Fatalf("got %q, want no line", got)
	}
	if rows := questRows(t, w.srv, w.player, q220Name); rows["<cond>"] != "8" {
		t.Fatalf("rows %v, want cond 8", rows)
	}
	if n := w.srv.PlayerItemCount(t, w.player, q220ScepterTantos); n != 0 {
		t.Fatalf("%d scepters of Tantos, want 0", n)
	}
}
