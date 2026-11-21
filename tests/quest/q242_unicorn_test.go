package quest

import (
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q242"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Possessor of a Precious Soul - 2's pure white unicorn (#5162): talked to
// at cond 10, the fallen unicorn is swapped three seconds later for a pure
// white unicorn that stays 30 s, one at a time for every player. However the
// pure white unicorn leaves, the next fallen unicorn swaps again. A scenario
// cannot show a script-spawned NPC appearing or leaving (#4020 item 5), so
// these Boot tests pin it; the unicorn scenario plays the answered dialog.
const (
	q242Name     = "Q242_PossessorOfAPreciousSoul"
	q242Path     = "quest.Q242_PossessorOfAPreciousSoul"
	q242Fallen   = 31746
	q242Pure     = 31747
	q242Restrain = 27317
)

// q242World boots a level 76 Warrior playing a level 60 Sorcerer subclass
// whose Possessor of a Precious Soul - 2 is at cond 10, with a fallen
// unicorn standing by it.
func q242World(t *testing.T) *dialogWorld {
	t.Helper()
	all := chatPages(questPages(q242Name, map[string][]string{
		"31746-02.htm": nil,
		"31747-01.htm": nil,
		"31747-02.htm": nil,
	}), q242Fallen, q242Pure)
	all["test/any.htm"] = anyBypassPage
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{7590, 7595, 7596, 7677, 7678} {
		templates = append(templates, &item.Template{
			ID: id, Name: "Quest Item " + strconv.Itoa(int(id)), Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	folk := []int32{31742, 31743, 31744, 31751, 31752, 30759, 30738, 31748, q242Fallen, q242Pure}
	kinds := map[int32]script.NPCKind{q242Restrain: script.KindHostile}
	npcs := []*npc.Template{proofMonster(q242Restrain)}
	for _, id := range folk {
		kinds[id] = script.KindFolk
		npcs = append(npcs, gameservertest.FolkTemplate("Folk", int(id)))
	}
	var classes []*player.Template
	for _, id := range []int{1, 2, 10, 11, 12} {
		tmpl := gameservertest.ClassTemplate()
		tmpl.ID = id
		classes = append(classes, tmpl)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithClassTemplates(classes...),
		gameservertest.WithClassCharacter("Seeker", 76, 0, 2),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithHTMLPages(all),
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithNPCs(npc.NewTable(npcs)),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: q242Path}}, script.Catalog{q242Path: q242.New}),
	)
	objID := srv.SoleObjectID(t)
	for _, q := range []string{
		"INSERT INTO character_subclasses (char_obj_id, class_id, exp, sp, level, class_index) VALUES (?, 12, 0, 0, 60, 1)",
		"UPDATE characters SET classid = 12 WHERE obj_Id = ?",
	} {
		if _, err := srv.DB.ExecContext(t.Context(), q, objID); err != nil {
			t.Fatalf("seed the subclass: %v", err)
		}
	}
	insertJournal(t, srv.DB,
		journalRow{objID, q242Name, "<state>", val("STARTED")},
		journalRow{objID, q242Name, "<cond>", val("10")},
	)
	enterWorld(t, srv)
	x, y, z := srv.PlayerPosition(t, objID)
	w := &dialogWorld{srv: srv, player: objID, at: location.Location{X: x, Y: y, Z: z}, roles: map[int32]string{}, npcs: map[string]int32{}}
	w.roles[w.player] = "player"
	w.spawnFolk(t, "fallen", q242Fallen, 40, 0, 0)
	return w
}

// swap talks to the fallen unicorn role, expecting 31746-02.htm.
func (w *dialogWorld) q242Swap(t *testing.T, role string) {
	t.Helper()
	if got := w.talk(t, role); !shows(got, "31746-02.htm") {
		t.Fatalf("%s: got %q, want 31746-02.htm", role, got)
	}
}

// wantUnicorns fails unless n pure white unicorns stand in the world.
func (w *dialogWorld) wantUnicorns(t *testing.T, step string, n int) {
	t.Helper()
	if got := len(npcsOf(w.srv, q242Pure)); got != n {
		t.Fatalf("%s: %d pure white unicorns, want %d", step, got, n)
	}
}

// TestQ242UnansweredUnicornFreesTheSwap: a pure white unicorn left
// unanswered leaves on its own 30 s later, and the next fallen unicorn
// talked to swaps for a new one.
func TestQ242UnansweredUnicornFreesTheSwap(t *testing.T) {
	t.Parallel()
	w := q242World(t)
	w.q242Swap(t, "fallen")
	w.wantUnicorns(t, "the talk", 0)
	w.advance(t, 3*time.Second)
	w.wantUnicorns(t, "the swap", 1)
	if n := len(npcsOf(w.srv, q242Fallen)); n != 0 {
		t.Fatalf("%d fallen unicorns after the swap, want 0", n)
	}

	w.advance(t, 29*time.Second)
	w.wantUnicorns(t, "29 s on", 1)
	w.advance(t, 2*time.Second)
	w.wantUnicorns(t, "31 s on", 0)

	w.spawnFolk(t, "fallen again", q242Fallen, -40, 0, 0)
	w.q242Swap(t, "fallen again")
	w.advance(t, 3*time.Second)
	w.wantUnicorns(t, "the swap after the leave", 1)
}

// TestQ242LateTalkTimerSparesTheNextUnicorn: the player talks to the pure
// white unicorn two seconds before it leaves on its own; a fallen unicorn
// talked to just before swaps once it has gone, and the talk's timer,
// firing after, takes away nothing.
func TestQ242LateTalkTimerSparesTheNextUnicorn(t *testing.T) {
	t.Parallel()
	w := q242World(t)
	w.q242Swap(t, "fallen")
	w.advance(t, 3*time.Second)
	pure := calledAt(w, q242Pure, [3]int{w.at.X + 40, w.at.Y, w.at.Z})
	if len(pure) != 1 {
		t.Fatalf("%d pure white unicorns at the fallen one's spot, want 1", len(pure))
	}
	w.roles[pure[0].ObjectID()], w.npcs["pure"] = "pure", pure[0].ObjectID()
	w.spawnFolk(t, "fallen again", q242Fallen, -40, 0, 0)

	w.advance(t, 28*time.Second) // 31 s: the next swap's timer ends at 34 s.
	w.q242Swap(t, "fallen again")
	w.advance(t, time.Second) // 32 s: the talk's timer ends at 35 s.
	if got := w.talk(t, "pure"); !shows(got, "31747-01.htm") {
		t.Fatalf("pure: got %q, want 31747-01.htm", got)
	}
	if rows := questRows(t, w.srv, w.player, q242Name); rows["<cond>"] != "11" {
		t.Fatalf("rows %v, want cond 11", rows)
	}

	w.advance(t, 1500*time.Millisecond) // 33.5 s: the first one left.
	w.wantUnicorns(t, "the leave", 0)
	w.advance(t, time.Second) // 34.5 s: the next swap.
	w.wantUnicorns(t, "the next swap", 1)
	w.advance(t, time.Second) // 35.5 s: the talk's timer has fired.
	w.wantUnicorns(t, "the talk's timer", 1)
}

// TestQ242AnsweredUnicornLeaves: the pure white unicorn talked to at cond 10
// moves the quest to cond 11 and leaves three seconds later, as before.
func TestQ242AnsweredUnicornLeaves(t *testing.T) {
	t.Parallel()
	w := q242World(t)
	w.q242Swap(t, "fallen")
	w.advance(t, 3*time.Second)
	pure := calledAt(w, q242Pure, [3]int{w.at.X + 40, w.at.Y, w.at.Z})
	if len(pure) != 1 {
		t.Fatalf("%d pure white unicorns at the fallen one's spot, want 1", len(pure))
	}
	w.roles[pure[0].ObjectID()], w.npcs["pure"] = "pure", pure[0].ObjectID()
	if got := w.talk(t, "pure"); !shows(got, "31747-01.htm") {
		t.Fatalf("pure: got %q, want 31747-01.htm", got)
	}
	if rows := questRows(t, w.srv, w.player, q242Name); rows["<cond>"] != "11" {
		t.Fatalf("rows %v, want cond 11", rows)
	}
	w.advance(t, 2900*time.Millisecond)
	w.wantUnicorns(t, "2.9 s after the talk", 1)
	w.advance(t, 200*time.Millisecond)
	w.wantUnicorns(t, "3.1 s after the talk", 0)
}
