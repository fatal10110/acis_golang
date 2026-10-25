package quest

import (
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q115"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The Other Side of Truth's suspicious man (Q115_TheOtherSideOfTruth
// onAdvEvent "sculpture-06.htm" and "32022-02.htm", onTimer). A scenario
// cannot show a script-spawned NPC appearing, speaking or leaving (#4020
// item 5), so these Boot tests pin him.
const (
	q115Name      = "Q115_TheOtherSideOfTruth"
	q115Path      = "quest.Q115_TheOtherSideOfTruth"
	q115Kierre    = 32022
	q115Sculpture = 32079
	q115Stranger  = 32019
	q115Report    = 8082
)

// q115World boots a level 53 character whose The Other Side of Truth is
// at cond with the sculpture progress ex.
func q115World(t *testing.T, cond, ex int) *dialogWorld {
	t.Helper()
	pages := chatPages(questPages(q115Name, map[string][]string{
		"sculpture-05.htm": {"sculpture-06.htm"},
		"sculpture-06.htm": nil,
		"32022-01.htm":     {"32022-02.htm"},
		"32022-02.htm":     nil,
	}), q115Kierre, q115Sculpture)
	w := bootSeamQuest(t, 53, pages, func(srv *gameservertest.Server, objID int32) {
		insertJournal(t, srv.DB,
			journalRow{objID, q115Name, "<state>", val("STARTED")},
			journalRow{objID, q115Name, "<cond>", val(strconv.Itoa(cond))},
			journalRow{objID, q115Name, "ex", val(strconv.Itoa(ex))})
	},
		x1Items([]int32{q115Report}),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{gameservertest.FolkTemplate("Folk", q115Stranger)})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{q115Kierre: script.KindFolk, q115Sculpture: script.KindFolk, q115Stranger: script.KindFolk},
			[]script.Listing{{Path: q115Path}},
			script.Catalog{q115Path: q115.New},
		),
	)
	w.roles[w.player] = "player"
	return w
}

// wantStranger fails unless the step's lines show the page file and the
// one suspicious man, standing 50 east and 50 south of the player, saying
// line, and returns him named "stranger".
func (w *dialogWorld) wantStranger(t *testing.T, step string, got []string, file string, line int32) *script.NPC {
	t.Helper()
	wantQ115Page(t, step, got, file)
	man := w.spawned(t, "stranger", q115Stranger)
	// The line renders before the man is named: it reads as his object id.
	wantLines(t, step, got, say(strconv.Itoa(int(man.ObjectID())), q115Stranger, line))
	// The engine places him on the ground below the given height.
	if loc := man.SpawnLocation(); loc.X != w.at.X+50 || loc.Y != w.at.Y+50 {
		t.Fatalf("%s: the suspicious man stands at %+v, want 50 east and south of %+v", step, loc, w.at)
	}
	return man
}

// wantQ115Page fails unless got shows the stand-in page file.
func wantQ115Page(t *testing.T, step string, got []string, file string) {
	t.Helper()
	if !shows(got, file) {
		t.Fatalf("%s: got %q, want the page %s", step, got, file)
	}
}

// wantStrangerLeaves lets the man say his parting line once said has
// passed and leave once gone has, checked 100 ms either side.
func (w *dialogWorld) wantStrangerLeaves(t *testing.T, said, gone time.Duration, line int32) {
	t.Helper()
	saysNone(t, "before the parting line", w.advance(t, said-100*time.Millisecond))
	wantLines(t, "the parting line", w.advance(t, 150*time.Millisecond), say("stranger", q115Stranger, line))
	if n := len(npcsOf(w.srv, q115Stranger)); n != 1 {
		t.Fatalf("%d suspicious men after the parting line, want 1", n)
	}
	saysNone(t, "the despawn", w.advance(t, gone-said))
	if n := len(npcsOf(w.srv, q115Stranger)); n != 0 {
		t.Fatalf("%d suspicious men after the despawn, want 0", n)
	}
}

// TestQ115StrangerWatchesTheFourthSculpture: at the fourth sculpture, with
// the first three passed, the quest moves to cond 8 and a suspicious man
// appears beside the player with a line; 3 s later he says another, and he
// is gone at 3.1 s.
func TestQ115StrangerWatchesTheFourthSculpture(t *testing.T) {
	t.Parallel()
	w := q115World(t, 7, 7)
	w.spawnFolk(t, "sculpture", q115Sculpture, 40, 0, 0)

	wantQ115Page(t, "the fourth sculpture", w.talk(t, "sculpture"), "sculpture-05.htm")
	got := w.send(t, encodeBypass("Quest "+q115Name+" sculpture-06.htm"))
	w.wantStranger(t, "the watcher", got, "sculpture-06.htm", 11550)
	if cond := questRows(t, w.srv, w.player, q115Name)["<cond>"]; cond != "8" {
		t.Fatalf("cond %q after the fourth sculpture, want 8", cond)
	}
	w.wantStrangerLeaves(t, 3000*time.Millisecond, 3100*time.Millisecond, 11551)
}

// TestQ115StrangerFollowsToKierre: Kierre's report moves the quest to
// cond 9 and gives the report piece, and the suspicious man appears again
// with a line; 5 s later he says another, and he is gone at 5.1 s.
func TestQ115StrangerFollowsToKierre(t *testing.T) {
	t.Parallel()
	w := q115World(t, 8, 7)
	w.spawnFolk(t, "kierre", q115Kierre, 40, 0, 0)

	wantQ115Page(t, "Kierre", w.talk(t, "kierre"), "32022-01.htm")
	got := w.send(t, encodeBypass("Quest "+q115Name+" 32022-02.htm"))
	w.wantStranger(t, "the report", got, "32022-02.htm", 11552)
	if cond := questRows(t, w.srv, w.player, q115Name)["<cond>"]; cond != "9" {
		t.Fatalf("cond %q after the report, want 9", cond)
	}
	if n := w.srv.PlayerItemCount(t, w.player, q115Report); n != 1 {
		t.Fatalf("report pieces %d, want 1", n)
	}
	w.wantStrangerLeaves(t, 5000*time.Millisecond, 5100*time.Millisecond, 11553)
}
