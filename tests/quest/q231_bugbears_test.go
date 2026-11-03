package quest

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q231"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Test of the Maestro's King Bugbears (Q231_TestOfTheMaestro onAdvEvent
// "30556-05.htm", onTimer "spawn_bugbears"): Toma's button teleports the
// player and, five seconds later, calls three King Bugbears around the spot,
// each set on the player and gone 200 s after it appeared. A scenario cannot
// show a script-spawned NPC appearing or leaving, nor act on one (#4020 item
// 5), so this Boot test pins them; the Toma scenario plays the dialog and the
// teleport.
const (
	q231Name    = "Q231_TestOfTheMaestro"
	q231Path    = "quest.Q231_TestOfTheMaestro"
	q231Toma    = 30556
	q231Bugbear = 20150

	q231PaintOfTeleportDevice = 2871
	q231BrokenTeleportDevice  = 2916
)

// Where the button teleports the player and where the bugbears stand.
var (
	q231Teleport  = [3]int{140352, -194133, -3146}
	q231BugbearAt = [3][3]int{{140333, -194153, -3138}, {140395, -194147, -3146}, {140304, -194082, -3157}}
)

// TestQ231TomaButtonCallsThreeBugbears: the button takes the paint for the
// broken device, teleports the player, and five seconds later three King
// Bugbears stand at their spots hating the player most, each gone 200 s after
// it appeared.
func TestQ231TomaButtonCallsThreeBugbears(t *testing.T) {
	t.Parallel()
	pages := chatPages(questPages(q231Name, map[string][]string{
		"30556-01.htm": {"30556-05.htm"},
		"30556-05.htm": nil,
	}), q231Toma)
	w := bootSeamQuest(t, 39, pages, func(srv *gameservertest.Server, objID int32) {
		insertJournal(t, srv.DB,
			journalRow{objID, q231Name, "<state>", val("STARTED")},
			journalRow{objID, q231Name, "<cond>", val("1")},
			journalRow{objID, q231Name, "aCond", val("1")})
		srv.GiveItem(t, objID, q231PaintOfTeleportDevice, 1)
	},
		x1Items([]int32{q231PaintOfTeleportDevice, q231BrokenTeleportDevice}),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			proofMonster(q231Bugbear), gameservertest.FolkTemplate("Folk", q231Toma),
		})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{
				q231Bugbear: script.KindHostile, q231Toma: script.KindFolk,
				// The rest of the quest's talkers, so its talk bindings register.
				30531: script.KindFolk, 30532: script.KindFolk, 30533: script.KindFolk, 30534: script.KindFolk,
				30535: script.KindFolk, 30536: script.KindFolk, 30671: script.KindFolk, 30672: script.KindFolk,
				30673: script.KindFolk,
			},
			[]script.Listing{{Path: q231Path}},
			script.Catalog{q231Path: q231.New},
		),
	)
	w.roles[w.player] = "player"
	w.spawnFolk(t, "toma", q231Toma, 40, 0, 0)

	if got := w.talk(t, "toma"); !shows(got, "30556-01.htm") {
		t.Fatalf("Toma: got %q, want 30556-01.htm", got)
	}
	if got := w.send(t, encodeBypass("Quest "+q231Name+" 30556-05.htm")); !shows(got, "30556-05.htm") {
		t.Fatalf("the button: got %q, want 30556-05.htm", got)
	}
	if x, y, z := w.srv.PlayerPosition(t, w.player); [3]int{x, y, z} != q231Teleport {
		t.Fatalf("the player stands at %d,%d,%d, want %v", x, y, z, q231Teleport)
	}
	if n := w.srv.PlayerItemCount(t, w.player, q231PaintOfTeleportDevice); n != 0 {
		t.Fatalf("paints of the teleport device %d, want 0", n)
	}
	if n := w.srv.PlayerItemCount(t, w.player, q231BrokenTeleportDevice); n != 1 {
		t.Fatalf("broken teleport devices %d, want 1", n)
	}

	w.advance(t, 4999*time.Millisecond)
	if n := len(npcsOf(w.srv, q231Bugbear)); n != 0 {
		t.Fatalf("%d bugbears 4999 ms after the button, want 0", n)
	}
	w.advance(t, 2*time.Millisecond)
	for _, at := range q231BugbearAt {
		called := calledAt(w, q231Bugbear, at)
		if len(called) != 1 {
			t.Fatalf("%d bugbears at %v, want 1", len(called), at)
		}
		if h := called[0].SpawnLocation().Heading; h != 0 {
			t.Fatalf("the bugbear at %v has heading %d, want 0", at, h)
		}
		if h := called[0].MostHated(); h == nil || h.ObjectID() != w.player {
			t.Fatalf("the bugbear at %v hates %v most, want the player", at, h)
		}
	}
	if n := len(npcsOf(w.srv, q231Bugbear)); n != 3 {
		t.Fatalf("%d bugbears called, want 3", n)
	}

	w.advance(t, 199*time.Second)
	if n := len(npcsOf(w.srv, q231Bugbear)); n != 3 {
		t.Fatalf("%d bugbears 199 s after they appeared, want 3", n)
	}
	w.advance(t, 2*time.Second)
	if n := len(npcsOf(w.srv, q231Bugbear)); n != 0 {
		t.Fatalf("%d bugbears after the despawn, want 0", n)
	}
}
