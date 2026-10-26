package quest

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q409"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The monsters Allana and Perrin call in Path to an Elven Oracle
// (Q409_PathToAnElvenOracle onAdvEvent, onTalk, onAttacked, onMyDying,
// onDecayed): Allana calls three lizardmen and, while any stands, says
// nothing; Perrin calls Tamil and, while it stands, repeats its page. A
// scenario cannot show a script-spawned NPC appearing, nor act on one
// (#4020 item 5), so these Boot tests pin the calls, the lines and the
// bookkeeping of the called monsters; the scenarios play the dialogs, the
// items and the fixture monsters' hooks.
const (
	q409Name = "Q409_PathToAnElvenOracle"
	q409Path = "quest.Q409_PathToAnElvenOracle"

	q409Manuel = 30293
	q409Allana = 30424
	q409Perrin = 30428

	q409Warrior = 27032
	q409Scout   = 27033
	q409Lizard  = 27034
	q409Tamil   = 27035

	q409Medallion = 1231
	q409Order     = 1234
	q409Diary     = 1236
	q409Necklace  = 1275
)

// q409World boots a character of level 19 whose quest is STARTED at cond,
// holding holds, with Allana and Perrin standing 40 and 60 east of it.
func q409World(t *testing.T, cond int, holds map[int32]int32) *dialogWorld {
	t.Helper()
	quest := q409.New()
	var templates []*npc.Template
	kinds := map[int32]script.NPCKind{}
	for _, id := range quest.Bind[script.EventTalked] {
		templates = append(templates, gameservertest.FolkTemplate("Folk", int(id)))
		kinds[id] = script.KindFolk
	}
	for _, id := range quest.Bind[script.EventMyDying] {
		templates = append(templates, proofMonster(int(id)))
		kinds[id] = script.KindHostile
	}
	pages := questPages(q409Name, map[string][]string{
		"30424-01.htm": {"spawn_lizards"}, "30424-02.htm": nil, "30424-03.htm": nil,
		"30428-01.htm": {"30428-06.htm"}, "30428-04.htm": nil, "30428-06.htm": nil,
	})
	w := bootSeamQuest(t, 19, chatPages(pages, q409Allana, q409Perrin), func(srv *gameservertest.Server, objID int32) {
		insertJournal(t, srv.DB,
			journalRow{objID, q409Name, "<state>", val("STARTED")},
			journalRow{objID, q409Name, "<cond>", val(strconv.Itoa(cond))})
		for id, count := range holds {
			srv.GiveItem(t, objID, id, count)
		}
	},
		x1Items(quest.Items),
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: q409Path}}, script.Catalog{q409Path: q409.New}),
		gameservertest.WithScriptRand((&scriptDraws{}).rand),
	)
	w.roles[w.player] = "player"
	w.spawnFolk(t, "allana", q409Allana, 40, 0, 0)
	w.spawnFolk(t, "perrin", q409Perrin, 60, 0, 0)
	return w
}

// q409Called checks that count NPCs of id stand in the world and returns them.
func q409Called(t *testing.T, w *dialogWorld, step string, id int32, count int) []*script.NPC {
	t.Helper()
	got := npcsOf(w.srv, id)
	if len(got) != count {
		t.Fatalf("%s: %d NPCs %d, want %d", step, len(got), id, count)
	}
	return got
}

// q409HasPage reports whether lines show any page.
func q409HasPage(lines []string) bool {
	return slices.ContainsFunc(lines, func(l string) bool { return strings.HasPrefix(l, "S NpcHtmlMessage ") })
}

// q409Cond asserts the quest's cond.
func q409Cond(t *testing.T, w *dialogWorld, want int) {
	t.Helper()
	if got := questRows(t, w.srv, w.player, q409Name)["<cond>"]; got != strconv.Itoa(want) {
		t.Fatalf("cond = %q, want %d", got, want)
	}
}

// q409Event sends the quest's bypass event and renders the answer.
func (w *dialogWorld) q409Event(t *testing.T, event string) []string {
	t.Helper()
	return w.send(t, encodeBypass("Quest "+q409Name+" "+event))
}

// TestQ409AllanaCallsThreeLizardmenAndWaitsForThem: Allana's event sets cond
// 2 and calls the warrior, the scout and the lizardman within 100 of her,
// with no page; while any of them stands, her talk shows no page; once all
// three have left, it shows her first page again.
func TestQ409AllanaCallsThreeLizardmenAndWaitsForThem(t *testing.T) {
	t.Parallel()
	w := q409World(t, 1, map[int32]int32{q409Medallion: 1})
	if got := w.talk(t, "allana"); !shows(got, "30424-01.htm") {
		t.Fatalf("first talk: got %q, want 30424-01.htm", got)
	}
	got := w.q409Event(t, "spawn_lizards")
	if q409HasPage(got) {
		t.Fatalf("the call shows a page: %q", got)
	}
	q409Cond(t, w, 2)
	var called []*script.NPC
	for _, id := range []int32{q409Warrior, q409Scout, q409Lizard} {
		n := q409Called(t, w, "after the call", id, 1)[0]
		if loc := n.SpawnLocation(); loc.X < w.at.X+40-100 || loc.X > w.at.X+40+100 || loc.Y < w.at.Y-100 || loc.Y > w.at.Y+100 {
			t.Fatalf("NPC %d stands at %+v, want within 100 of Allana beside %+v", id, loc, w.at)
		}
		called = append(called, n)
	}

	for i, n := range called {
		if got := w.talk(t, "allana"); q409HasPage(got) {
			t.Fatalf("talk with %d lizardmen standing: got %q, want no page", 3-i, got)
		}
		n.DeleteMe()
		w.advance(t, time.Second)
	}
	if got := w.talk(t, "allana"); !shows(got, "30424-01.htm") {
		t.Fatalf("talk with no lizardman standing: got %q, want 30424-01.htm", got)
	}
}

// TestQ409FirstValidHitMakesTheLizardmanSpeakAndOnlyItsDeathCounts: a called
// lizardman says its line at the first hit of the character and not at the
// next; its death at cond 2 makes the captain say his, gives the order and
// sets cond 3; the death of a lizardman marked invalid does nothing.
func TestQ409FirstValidHitMakesTheLizardmanSpeakAndOnlyItsDeathCounts(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		id      int32
		said    int32
		invalid bool
	}{
		{"warrior", q409Warrior, 40909, false},
		{"scout", q409Scout, 40911, false},
		{"lizardman", q409Lizard, 40912, false},
		{"invalid", q409Warrior, 40909, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := q409World(t, 1, map[int32]int32{q409Medallion: 1})
			w.talk(t, "allana")
			w.q409Event(t, "spawn_lizards")
			n := q409Called(t, w, "after the call", tc.id, 1)[0]
			w.roles[n.ObjectID()] = "lizard"
			h := hostileNPC(t, w.srv, n)

			hit(t, w.srv, w.player, h, 5)
			wantLines(t, "first hit", w.advance(t, 500*time.Millisecond), say("lizard", tc.id, tc.said))
			if v := n.ScriptValue(); v != w.player {
				t.Fatalf("script value %d after the first hit, want the player %d", v, w.player)
			}
			hit(t, w.srv, w.player, h, 5)
			saysNone(t, "second hit", w.advance(t, 500*time.Millisecond))

			if tc.invalid {
				n.SetScriptValue(-1)
			}
			killOnPlayerQueue(t, w.srv, w.player, h)
			got := w.render(t, w.srv.ReadQueued(t, w.srv.Client))
			if tc.invalid {
				saysNone(t, "death of an invalid lizardman", got)
				q409Cond(t, w, 2)
				q409Items(t, w, q409Order, 0)
				return
			}
			wantLines(t, "death", got, say("lizard", tc.id, 40910))
			q409Cond(t, w, 3)
			q409Items(t, w, q409Order, 1)
		})
	}
}

// q409Items checks the count of an item the character holds.
func q409Items(t *testing.T, w *dialogWorld, item int32, want int) {
	t.Helper()
	if n := w.srv.PlayerItemCount(t, w.player, item); n != want {
		t.Fatalf("item %d held %d times, want %d", item, n, want)
	}
}

// TestQ409PerrinCallsTamilAndRepeatsHisPageWhileHeStands: Perrin's event at
// cond 4 calls Tamil within 100 of him and shows 30428-06; while Tamil
// stands, the talk shows 30428-06; once it has left, 30428-01. Tamil says
// his line at the first hit, and his death sets cond 5 and gives the
// necklace.
func TestQ409PerrinCallsTamilAndRepeatsHisPageWhileHeStands(t *testing.T) {
	t.Parallel()
	w := q409World(t, 4, map[int32]int32{q409Medallion: 1, q409Order: 1, q409Diary: 1})
	if got := w.talk(t, "perrin"); !shows(got, "30428-01.htm") {
		t.Fatalf("first talk: got %q, want 30428-01.htm", got)
	}
	if got := w.q409Event(t, "30428-06.htm"); !shows(got, "30428-06.htm") {
		t.Fatalf("Perrin's event: got %q, want 30428-06.htm", got)
	}
	tamil := q409Called(t, w, "after the call", q409Tamil, 1)[0]
	if loc := tamil.SpawnLocation(); loc.X < w.at.X+60-100 || loc.X > w.at.X+60+100 || loc.Y < w.at.Y-100 || loc.Y > w.at.Y+100 {
		t.Fatalf("Tamil stands at %+v, want within 100 of Perrin beside %+v", loc, w.at)
	}
	if got := w.talk(t, "perrin"); !shows(got, "30428-06.htm") {
		t.Fatalf("talk with Tamil standing: got %q, want 30428-06.htm", got)
	}

	tamil.DeleteMe()
	w.advance(t, time.Second)
	if got := w.talk(t, "perrin"); !shows(got, "30428-01.htm") {
		t.Fatalf("talk with Tamil gone: got %q, want 30428-01.htm", got)
	}

	w.q409Event(t, "30428-06.htm")
	tamil = q409Called(t, w, "after the second call", q409Tamil, 1)[0]
	w.roles[tamil.ObjectID()] = "tamil"
	h := hostileNPC(t, w.srv, tamil)
	hit(t, w.srv, w.player, h, 5)
	wantLines(t, "first hit", w.advance(t, 500*time.Millisecond), say("tamil", q409Tamil, 40913))
	killOnPlayerQueue(t, w.srv, w.player, h)
	w.render(t, w.srv.ReadQueued(t, w.srv.Client))
	q409Cond(t, w, 5)
	q409Items(t, w, q409Necklace, 1)
}
