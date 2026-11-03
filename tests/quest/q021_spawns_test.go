package quest

import (
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q021"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Hidden Truth's duke and page (Q021_HiddenTruth onAdvEvent "31523-03.htm",
// "31524-06.htm" and "31526-08.htm", onTimer, onDecayed). A scenario cannot
// show a script-spawned NPC appearing, speaking from out of sight or leaving
// (#4020 item 5), so this Boot test pins them.
const (
	q021Name      = "Q021_HiddenTruth"
	q021Path      = "quest.Q021_HiddenTruth"
	q021Tombstone = 31523
	q021Duke      = 31524
	q021Page      = 31525
	q021Bookshelf = 31526
)

// q021DukeAt is where the tombstone wakes the duke's ghost.
var q021DukeAt = [3]int{51432, -54570, -3136}

// TestQ021DukeCallsThePageWhoLeadsThePlayer: the tombstone wakes the duke's
// ghost, which speaks as it appears (cond 2); the duke calls his page, which
// offers to guide the player by name half a second later and has led it to
// the bookcase nine seconds after that, marking end_walk (cond 3). The
// bookshelf sends the page away at once (cond 5), and the duke leaves 300 s
// after he woke, without a word.
func TestQ021DukeCallsThePageWhoLeadsThePlayer(t *testing.T) {
	t.Parallel()
	pages := chatPages(questPages(q021Name, map[string][]string{
		"31523-01.htm": {"31523-03.htm"},
		"31523-03.htm": nil,
		"31524-01.htm": {"31524-06.htm"},
		"31524-06.htm": nil,
		"31526-01.htm": {"31526-08.htm"},
		"31526-08.htm": nil,
	}), q021Tombstone, q021Duke, q021Bookshelf)
	w := bootSeamQuest(t, 63, pages, func(srv *gameservertest.Server, objID int32) {
		if _, err := srv.DB.Exec(`UPDATE characters SET x = ?, y = ?, z = ? WHERE obj_Id = ?`,
			q021DukeAt[0], q021DukeAt[1], q021DukeAt[2], objID); err != nil {
			t.Fatal(err)
		}
		insertJournal(t, srv.DB,
			journalRow{objID, q021Name, "<state>", val("STARTED")},
			journalRow{objID, q021Name, "<cond>", val("1")})
	},
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			gameservertest.FolkTemplate("Folk", q021Duke), gameservertest.FolkTemplate("Folk", q021Page),
		})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{q021Tombstone: script.KindFolk, q021Duke: script.KindFolk, q021Page: script.KindFolk, q021Bookshelf: script.KindFolk},
			[]script.Listing{{Path: q021Path}},
			script.Catalog{q021Path: q021.New},
		),
	)
	w.roles[w.player] = "player"
	w.spawnFolk(t, "tombstone", q021Tombstone, 40, 0, 0)
	w.spawnFolk(t, "bookshelf", q021Bookshelf, -40, 0, 0)

	if got := w.talk(t, "tombstone"); !shows(got, "31523-01.htm") {
		t.Fatalf("tombstone: got %q, want 31523-01.htm", got)
	}
	got := w.send(t, encodeBypass("Quest "+q021Name+" 31523-03.htm"))
	duke := w.spawned(t, "duke", q021Duke)
	// The duke speaks before he is named: he reads as his object id.
	wantLines(t, "the wake", got, say(strconv.Itoa(int(duke.ObjectID())), q021Duke, 2150),
		"S PlaySound type=0 file=SkillSound5.horror_02 bind=0 obj=0 loc=0,0,0 delay=0")
	if !shows(got, "31523-03.htm") {
		t.Fatalf("the wake: got %q, want 31523-03.htm", got)
	}
	if loc := duke.SpawnLocation(); loc.X != q021DukeAt[0] || loc.Y != q021DukeAt[1] || abs(loc.Z-q021DukeAt[2]) > 32 {
		t.Fatalf("the duke stands at %+v, want %v", loc, q021DukeAt)
	}
	if cond := questRows(t, w.srv, w.player, q021Name)["<cond>"]; cond != "2" {
		t.Fatalf("cond %q after the wake, want 2", cond)
	}

	if got := w.talk(t, "duke"); !shows(got, "31524-01.htm") {
		t.Fatalf("duke: got %q, want 31524-01.htm", got)
	}
	got = w.send(t, encodeBypass("Quest "+q021Name+" 31524-06.htm"))
	if !shows(got, "31524-06.htm") {
		t.Fatalf("the call: got %q, want 31524-06.htm", got)
	}
	w.spawned(t, "page", q021Page)
	saysNone(t, "the call", got)
	wantLines(t, "half a second later", w.advance(t, 500*time.Millisecond), say("page", q021Page, 2151, "Seeker"))
	saysNone(t, "8.9 s later", w.advance(t, 8900*time.Millisecond))
	wantLines(t, "the walk's end", w.advance(t, 100*time.Millisecond), say("page", q021Page, 2152, "Seeker"))
	if rows := questRows(t, w.srv, w.player, q021Name); rows["<cond>"] != "3" || rows["end_walk"] != "1" {
		t.Fatalf("rows %v after the walk, want cond 3 and end_walk 1", rows)
	}

	if got := w.talk(t, "bookshelf"); !shows(got, "31526-01.htm") {
		t.Fatalf("bookshelf: got %q, want 31526-01.htm", got)
	}
	w.send(t, encodeBypass("Quest "+q021Name+" 31526-08.htm"))
	if n := len(npcsOf(w.srv, q021Page)); n != 0 {
		t.Fatalf("%d pages after the bookshelf sent it away, want 0", n)
	}
	if rows := questRows(t, w.srv, w.player, q021Name); rows["<cond>"] != "5" || rows["end_walk"] != "" {
		t.Fatalf("rows %v after the bookshelf, want cond 5 and no end_walk", rows)
	}

	saysNone(t, "298.5 s after the wake", w.advance(t, 289*time.Second))
	if n := len(npcsOf(w.srv, q021Duke)); n != 1 {
		t.Fatalf("%d dukes 298.5 s after the wake, want 1", n)
	}
	saysNone(t, "the duke's despawn", w.advance(t, 2*time.Second))
	if n := len(npcsOf(w.srv, q021Duke)); n != 0 {
		t.Fatalf("%d dukes after the despawn, want 0", n)
	}
}
