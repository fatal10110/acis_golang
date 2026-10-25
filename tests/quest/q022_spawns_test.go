package quest

import (
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q022"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Tragedy in von Hellmann Forest's ghost of the priest and soul of the well
// (Q022_TragedyInVonHellmannForest onAdvEvent "31334-13.htm", "31528-08.htm"
// and "31527-02.htm", onTimer, onAttacked, onDecayed, onMyDying). A scenario
// cannot show a script-spawned NPC appearing, speaking from out of sight or
// leaving, nor act on one (#4020 item 5), so these Boot tests pin them.
const (
	q022Name       = "Q022_TragedyInVonHellmannForest"
	q022Path       = "quest.Q022_TragedyInVonHellmannForest"
	q022Tifaren    = 31334
	q022Priest     = 31528
	q022Well       = 31527
	q022Soul       = 27217
	q022Skull      = 7142
	q022GreenJewel = 7144
	q022RedJewel   = 7145
)

var (
	// q022GhostAt is where Tifaren calls the ghost of the priest.
	q022GhostAt = [3]int{38418, -49894, -1104}
	// q022SoulAt is where the well wakes its soul.
	q022SoulAt = [3]int{34860, -54542, -2048}
)

// q022World boots a level 63 character whose Tragedy in von Hellmann Forest
// is at cond, holding one of each of holds, standing at at.
func q022World(t *testing.T, at [3]int, cond int, pages map[string][]string, holds ...int32) *dialogWorld {
	t.Helper()
	w := bootSeamQuest(t, 63, chatPages(questPages(q022Name, pages), q022Tifaren, q022Priest, q022Well), func(srv *gameservertest.Server, objID int32) {
		if _, err := srv.DB.Exec(`UPDATE characters SET x = ?, y = ?, z = ? WHERE obj_Id = ?`, at[0], at[1], at[2], objID); err != nil {
			t.Fatal(err)
		}
		insertJournal(t, srv.DB,
			journalRow{objID, q022Name, "<state>", val("STARTED")},
			journalRow{objID, q022Name, "<cond>", val(strconv.Itoa(cond))})
		for _, id := range holds {
			srv.GiveItem(t, objID, id, 1)
		}
	},
		x1Items([]int32{q022Skull, q022GreenJewel, q022RedJewel}),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{gameservertest.FolkTemplate("Folk", q022Priest), proofMonster(q022Soul)})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{q022Tifaren: script.KindFolk, q022Priest: script.KindFolk, q022Well: script.KindFolk, q022Soul: script.KindHostile},
			[]script.Listing{{Path: q022Path}},
			script.Catalog{q022Path: q022.New},
		),
	)
	w.roles[w.player] = "player"
	return w
}

// TestQ022GhostOfThePriestLeavesThreeSecondsAfterItsStory: Tifaren calls the
// ghost of the priest for the skull, and it calls the player out by name
// (cond 7); while it stands Tifaren says so. Three seconds after its story
// (cond 8) it leaves, with its last word to those around it.
func TestQ022GhostOfThePriestLeavesThreeSecondsAfterItsStory(t *testing.T) {
	t.Parallel()
	w := q022World(t, q022GhostAt, 5, map[string][]string{
		"31334-10.htm": {"31334-13.htm"},
		"31334-13.htm": nil,
		"31334-15.htm": nil,
		"31528-01.htm": {"31528-08.htm"},
		"31528-08.htm": nil,
	}, q022Skull)
	w.spawnFolk(t, "tifaren", q022Tifaren, 40, 0, 0)

	if got := w.talk(t, "tifaren"); !shows(got, "31334-10.htm") {
		t.Fatalf("Tifaren: got %q, want 31334-10.htm", got)
	}
	got := w.send(t, encodeBypass("Quest "+q022Name+" 31334-13.htm"))
	ghost := w.spawned(t, "ghost", q022Priest)
	wantLines(t, "the call", got, say(strconv.Itoa(int(ghost.ObjectID())), q022Priest, 2250, "Seeker"))
	if !shows(got, "31334-13.htm") {
		t.Fatalf("the call: got %q, want 31334-13.htm", got)
	}
	if loc := ghost.SpawnLocation(); loc.X != q022GhostAt[0] || loc.Y != q022GhostAt[1] || abs(loc.Z-q022GhostAt[2]) > 32 {
		t.Fatalf("the ghost stands at %+v, want %v", loc, q022GhostAt)
	}
	if rows := questRows(t, w.srv, w.player, q022Name); rows["<cond>"] != "7" {
		t.Fatalf("rows %v after the call, want cond 7", rows)
	}
	if n := w.srv.PlayerItemCount(t, w.player, q022Skull); n != 0 {
		t.Fatalf("skulls %d after the call, want 0", n)
	}
	if got := w.talk(t, "tifaren"); !shows(got, "31334-15.htm") {
		t.Fatalf("Tifaren while the ghost stands: got %q, want 31334-15.htm", got)
	}

	if got := w.talk(t, "ghost"); !shows(got, "31528-01.htm") {
		t.Fatalf("ghost: got %q, want 31528-01.htm", got)
	}
	w.send(t, encodeBypass("Quest "+q022Name+" 31528-08.htm"))
	if rows := questRows(t, w.srv, w.player, q022Name); rows["<cond>"] != "8" {
		t.Fatalf("rows %v after the story, want cond 8", rows)
	}
	saysNone(t, "2.9 s after the story", w.advance(t, 2900*time.Millisecond))
	if n := len(npcsOf(w.srv, q022Priest)); n != 1 {
		t.Fatalf("%d ghosts 2.9 s after the story, want 1", n)
	}
	wantLines(t, "the departure", w.advance(t, 100*time.Millisecond), say("ghost", q022Priest, 2251))
	if n := len(npcsOf(w.srv, q022Priest)); n != 0 {
		t.Fatalf("%d ghosts after the departure, want 0", n)
	}
}

// soulWorld boots a character at cond 10 holding the green jewel, standing
// where the well wakes its soul, and wakes it.
func soulWorld(t *testing.T) (*dialogWorld, *npc.Hostile) {
	t.Helper()
	w := q022World(t, q022SoulAt, 10, map[string][]string{
		"31527-01.htm": {"31527-02.htm"},
		"31527-02.htm": nil,
	}, q022GreenJewel)
	w.spawnFolk(t, "well", q022Well, 40, 0, 0)
	if got := w.talk(t, "well"); !shows(got, "31527-01.htm") {
		t.Fatalf("well: got %q, want 31527-01.htm", got)
	}
	if got := w.send(t, encodeBypass("Quest "+q022Name+" 31527-02.htm")); !shows(got, "31527-02.htm") {
		t.Fatalf("the wake: got %q, want 31527-02.htm", got)
	}
	soul := w.spawned(t, "soul", q022Soul)
	if h := soul.MostHated(); h == nil || h.ObjectID() != w.player {
		t.Fatalf("the soul hates %v most, want the player", h)
	}
	return w, hostileNPC(t, w.srv, soul)
}

// TestQ022SoulOfTheWellHeldTwentySeconds: the player's hit on the soul of
// the well at cond 10 starts twenty seconds, after which the green jewel
// turns red (cond 11). A soul killed before then stops them, and frees the
// well, which wakes another.
func TestQ022SoulOfTheWellHeldTwentySeconds(t *testing.T) {
	t.Parallel()
	t.Run("held", func(t *testing.T) {
		t.Parallel()
		w, soul := soulWorld(t)
		hit(t, w.srv, w.player, soul, 10)
		w.advance(t, 19900*time.Millisecond)
		if rows := questRows(t, w.srv, w.player, q022Name); rows["<cond>"] != "10" {
			t.Fatalf("rows %v 19.9 s after the hit, want cond 10", rows)
		}
		wantLines(t, "twenty seconds", w.advance(t, 100*time.Millisecond),
			"S PlaySound type=0 file=ItemSound.quest_middle bind=0 obj=0 loc=0,0,0 delay=0")
		if rows := questRows(t, w.srv, w.player, q022Name); rows["<cond>"] != "11" {
			t.Fatalf("rows %v after twenty seconds, want cond 11", rows)
		}
		if g, r := w.srv.PlayerItemCount(t, w.player, q022GreenJewel), w.srv.PlayerItemCount(t, w.player, q022RedJewel); g != 0 || r != 1 {
			t.Fatalf("green %d red %d after twenty seconds, want 0 and 1", g, r)
		}
	})
	t.Run("killed", func(t *testing.T) {
		t.Parallel()
		w, soul := soulWorld(t)
		hit(t, w.srv, w.player, soul, 10)
		killOnPlayerQueue(t, w.srv, w.player, soul)
		w.advance(t, 20*time.Second)
		if rows := questRows(t, w.srv, w.player, q022Name); rows["<cond>"] != "10" {
			t.Fatalf("rows %v after the kill and twenty seconds, want cond 10", rows)
		}
		if g := w.srv.PlayerItemCount(t, w.player, q022GreenJewel); g != 1 {
			t.Fatalf("green %d after the kill, want 1", g)
		}
		w.talk(t, "well")
		w.send(t, encodeBypass("Quest "+q022Name+" 31527-02.htm"))
		var alive int
		for _, n := range npcsOf(w.srv, q022Soul) {
			if !n.Dead() {
				alive++
			}
		}
		if alive != 1 {
			t.Fatalf("%d live souls after the second wake, want 1", alive)
		}
	})
}
