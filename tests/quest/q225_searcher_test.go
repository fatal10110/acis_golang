package quest

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q225"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Test of the Searcher's called NPCs (Q225_TestOfTheSearcher onAttacked,
// onAdvEvent "30627-01a.htm" and "30628-01a.htm", onDecayed): a Delu
// lizardman shaman hit by a holder of Leirynn's first order calls one Neer
// bodyguard for 200 s, and the tree calls the strong wooden chest, one at
// a time, which its own bypass removes. A scenario cannot show a
// script-spawned NPC appearing or leaving, nor act on one (#4020 item 5),
// so these Boot tests pin them; the scenarios play the dialogs.
const (
	q225Name          = "Q225_TestOfTheSearcher"
	q225Path          = "quest.Q225_TestOfTheSearcher"
	q225DeluShaman    = 20781
	q225NeerBodyguard = 27092
	q225Tree          = 30627
	q225Chest         = 30628
	q225LeirynnOrder1 = 2786
	q225RustedKey     = 2806
	q225GoldBar       = 2807
)

// q225ChestAt is where the tree calls the strong wooden chest.
var q225ChestAt = [3]int{10098, 157287, -2406}

// q225World boots a level 39 character whose Test of the Searcher is at
// cond, holding one of each of hold, with the tree and a strong wooden
// chest of its own (the role "chest") standing by it.
func q225World(t *testing.T, cond string, hold ...int32) *dialogWorld {
	t.Helper()
	pages := chatPages(questPages(q225Name, map[string][]string{
		"30627-01.htm":  {"30627-01a.htm"},
		"30627-01a.htm": nil,
		"30628-01.htm":  {"30628-01a.htm"},
		"30628-01a.htm": nil,
		"30628-02.htm":  nil,
	}), q225Tree, q225Chest)
	w := bootSeamQuest(t, 39, pages, func(srv *gameservertest.Server, objID int32) {
		insertJournal(t, srv.DB,
			journalRow{objID, q225Name, "<state>", val("STARTED")},
			journalRow{objID, q225Name, "<cond>", val(cond)})
		for _, id := range hold {
			srv.GiveItem(t, objID, id, 1)
		}
	},
		x1Items([]int32{q225LeirynnOrder1, q225RustedKey, q225GoldBar}),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			proofMonster(q225DeluShaman), proofMonster(q225NeerBodyguard),
			gameservertest.FolkTemplate("Folk", q225Chest),
		})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{
				q225DeluShaman: script.KindHostile, q225NeerBodyguard: script.KindHostile,
				q225Tree: script.KindFolk, q225Chest: script.KindFolk,
				// The rest of the quest's talkers, so its talk bindings register.
				30291: script.KindFolk, 30420: script.KindFolk, 30690: script.KindFolk, 30728: script.KindFolk,
				30729: script.KindFolk, 30730: script.KindFolk,
			},
			[]script.Listing{{Path: q225Path}},
			script.Catalog{q225Path: q225.New},
		),
	)
	w.roles[w.player] = "player"
	w.spawnFolk(t, "tree", q225Tree, 40, 0, 0)
	w.spawnFolk(t, "chest", q225Chest, -40, 0, 0)
	return w
}

// TestQ225ShamanCallsOneNeerBodyguard: a Delu lizardman shaman hit by a
// player holding Leirynn's first order calls one Neer bodyguard where it
// stands, with no offset, gone 200 s later; it calls once. Without the
// order a hit calls none.
func TestQ225ShamanCallsOneNeerBodyguard(t *testing.T) {
	t.Parallel()
	t.Run("order held", func(t *testing.T) {
		t.Parallel()
		w := q225World(t, "3", q225LeirynnOrder1)
		h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(q225DeluShaman), location.Location{X: w.at.X + 60, Y: w.at.Y, Z: w.at.Z})
		x, y, z := h.Position()
		hit(t, w.srv, w.player, h, 1)
		guards := npcsOf(w.srv, q225NeerBodyguard)
		if len(guards) != 1 {
			t.Fatalf("%d bodyguards after the hit, want 1", len(guards))
		}
		if loc := guards[0].SpawnLocation(); loc.X != x || loc.Y != y || abs(loc.Z-z) > 32 {
			t.Fatalf("the bodyguard stands at %+v, want the shaman's (%d, %d, %d)", loc, x, y, z)
		}
		hit(t, w.srv, w.player, h, 1)
		if n := len(npcsOf(w.srv, q225NeerBodyguard)); n != 1 {
			t.Fatalf("%d bodyguards after a second hit, want 1", n)
		}
		w.advance(t, 199*time.Second)
		if n := len(npcsOf(w.srv, q225NeerBodyguard)); n != 1 {
			t.Fatalf("%d bodyguards 199 s after the call, want 1", n)
		}
		w.advance(t, 2*time.Second)
		if n := len(npcsOf(w.srv, q225NeerBodyguard)); n != 0 {
			t.Fatalf("%d bodyguards after the despawn, want 0", n)
		}
	})
	t.Run("no order", func(t *testing.T) {
		t.Parallel()
		w := q225World(t, "3")
		h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(q225DeluShaman), location.Location{X: w.at.X + 60, Y: w.at.Y, Z: w.at.Z})
		hit(t, w.srv, w.player, h, 1)
		if n := len(npcsOf(w.srv, q225NeerBodyguard)); n != 0 {
			t.Fatalf("%d bodyguards, want 0", n)
		}
	})
}

// calledChests returns the strong wooden chests the tree called: those
// standing at its spot.
func calledChests(w *dialogWorld) []*script.NPC {
	var out []*script.NPC
	for _, n := range npcsOf(w.srv, q225Chest) {
		if loc := n.SpawnLocation(); loc.X == q225ChestAt[0] && loc.Y == q225ChestAt[1] && abs(loc.Z-q225ChestAt[2]) <= 32 {
			out = append(out, n)
		}
	}
	return out
}

// treeEvent talks to the tree and sends its bypass.
func (w *dialogWorld) treeEvent(t *testing.T) []string {
	t.Helper()
	if got := w.talk(t, "tree"); !shows(got, "30627-01.htm") {
		t.Fatalf("tree: got %q, want 30627-01.htm", got)
	}
	return w.send(t, encodeBypass("Quest "+q225Name+" 30627-01a.htm"))
}

// TestQ225TreeCallsOneChest: at cond 16 the tree gives the rusted key
// (cond 17) and calls the strong wooden chest at its spot; while that
// chest stands, the tree calls no other and gives nothing. The chest's
// bypass, with the key, gives twenty gold bars (cond 18) and removes the
// called chest.
func TestQ225TreeCallsOneChest(t *testing.T) {
	t.Parallel()
	w := q225World(t, "16")
	if got := w.treeEvent(t); !shows(got, "30627-01a.htm") {
		t.Fatalf("the tree's bypass: got %q, want 30627-01a.htm", got)
	}
	called := calledChests(w)
	if len(called) != 1 {
		t.Fatalf("%d chests called, want 1", len(called))
	}
	// The call passes 300000 where the heading goes, and no despawn time.
	if h := called[0].SpawnLocation().Heading; h != 300000 {
		t.Fatalf("the chest's heading is %d, want 300000", h)
	}
	if rows := questRows(t, w.srv, w.player, q225Name); rows["<cond>"] != "17" {
		t.Fatalf("rows %v after the call, want cond 17", rows)
	}
	w.treeEvent(t)
	if n := len(calledChests(w)); n != 1 {
		t.Fatalf("%d chests after a second call, want 1", n)
	}
	if n := w.srv.PlayerItemCount(t, w.player, q225RustedKey); n != 1 {
		t.Fatalf("rusted keys %d, want 1", n)
	}

	if got := w.talk(t, "chest"); !shows(got, "30628-01.htm") {
		t.Fatalf("chest: got %q, want 30628-01.htm", got)
	}
	if got := w.send(t, encodeBypass("Quest "+q225Name+" 30628-01a.htm")); !shows(got, "30628-01a.htm") {
		t.Fatalf("the chest's bypass: got %q, want 30628-01a.htm", got)
	}
	if n := len(calledChests(w)); n != 0 {
		t.Fatalf("%d called chests after the opening, want 0", n)
	}
	if n := w.srv.PlayerItemCount(t, w.player, q225GoldBar); n != 20 {
		t.Fatalf("gold bars %d, want 20", n)
	}
	if rows := questRows(t, w.srv, w.player, q225Name); rows["<cond>"] != "18" {
		t.Fatalf("rows %v after the opening, want cond 18", rows)
	}
}

// TestQ225ChestGoneLetsTheTreeCallAgain: a called chest that leaves the
// world (as the chest's bypass removes it) frees the tree to call another,
// with no second key once past cond 16.
func TestQ225ChestGoneLetsTheTreeCallAgain(t *testing.T) {
	t.Parallel()
	w := q225World(t, "16")
	w.treeEvent(t)
	called := calledChests(w)
	if len(called) != 1 {
		t.Fatalf("%d chests called, want 1", len(called))
	}
	// It never leaves on its own.
	w.advance(t, 10*time.Minute)
	if n := len(calledChests(w)); n != 1 {
		t.Fatalf("%d called chests ten minutes on, want 1", n)
	}
	called[0].DeleteMe()
	w.treeEvent(t)
	again := calledChests(w)
	if len(again) != 1 || again[0].ObjectID() == called[0].ObjectID() {
		t.Fatalf("called chests %v after the first left, want one new one", again)
	}
	if n := w.srv.PlayerItemCount(t, w.player, q225RustedKey); n != 1 {
		t.Fatalf("rusted keys %d after the second call, want 1", n)
	}
}
