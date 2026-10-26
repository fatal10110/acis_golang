package quest

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q232"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Test of the Lord's First Orc (Q232_TestOfTheLord onAdvEvent "30649-07.htm"
// and "30643-02.htm", onTimer "f_orc_despawn"): Martankus' page calls the
// First Orc at a spot, one at a time, and the orc's own page starts a
// ten-second timer that sends it away and lets Martankus call another. A
// scenario cannot show a script-spawned NPC appearing or leaving, nor act on
// one (#4020 item 5), so this Boot test pins them; the scenarios play the
// dialogs.
const (
	q232Name      = "Q232_TestOfTheLord"
	q232Path      = "quest.Q232_TestOfTheLord"
	q232Martankus = 30649
	q232FirstOrc  = 30643
)

// Where the dialog calls the First Orc, and the heading the quest gives it.
var q232FirstOrcAt = [3]int{21036, -107690, -3038}

const q232FirstOrcHeading = 200000

// q232World boots a level 39 character whose Test of the Lord is at cond,
// with Martankus standing by it.
func q232World(t *testing.T, cond string) *dialogWorld {
	t.Helper()
	pages := chatPages(questPages(q232Name, map[string][]string{
		"30649-06.htm": {"30649-07.htm"},
		"30649-07.htm": nil,
		"30649-08.htm": {"30649-07.htm"},
		"30643-01.htm": {"30643-02.htm"},
		"30643-02.htm": nil,
	}), q232Martankus, q232FirstOrc)
	talkers := []int32{30565, 30642, 30568, 30510, 30564, 30641, 30567, 30558, 30566, 30515}
	dying := []int32{20233, 20269, 20270, 20564, 20583, 20584, 20585, 20586, 20587, 20588, 20778, 20779}
	kinds := map[int32]script.NPCKind{q232Martankus: script.KindFolk, q232FirstOrc: script.KindFolk}
	templates := []*npc.Template{
		gameservertest.FolkTemplate("Folk", q232Martankus), gameservertest.FolkTemplate("Folk", q232FirstOrc),
	}
	for _, id := range talkers {
		kinds[id] = script.KindFolk // the rest of the quest's talkers and monsters, so its bindings register
	}
	for _, id := range dying {
		kinds[id] = script.KindHostile
	}
	w := bootSeamQuest(t, 39, pages, func(srv *gameservertest.Server, objID int32) {
		insertJournal(t, srv.DB,
			journalRow{objID, q232Name, "<state>", val("STARTED")},
			journalRow{objID, q232Name, "<cond>", val(cond)})
	},
		x1Items([]int32{3413, 3414, 3415, 3416}),
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: q232Path}}, script.Catalog{q232Path: q232.New}),
	)
	w.roles[w.player] = "player"
	w.spawnFolk(t, "martankus", q232Martankus, 40, 0, 0)
	return w
}

// TestQ232MartankusCallsOneFirstOrc: Martankus' event "30649-07.htm" calls
// the First Orc at its spot with the heading 200000, and while it stands
// calls no other; the orc's event "30643-02.htm" sets cond 7 and, ten seconds
// later, sends it away, after which Martankus calls a new one.
func TestQ232MartankusCallsOneFirstOrc(t *testing.T) {
	t.Parallel()
	w := q232World(t, "5")
	if got := w.talk(t, "martankus"); !shows(got, "30649-06.htm") {
		t.Fatalf("Martankus: got %q, want 30649-06.htm", got)
	}
	if rows := questRows(t, w.srv, w.player, q232Name); rows["<cond>"] != "6" {
		t.Fatalf("rows %v, want cond 6", rows)
	}
	if n := len(npcsOf(w.srv, q232FirstOrc)); n != 0 {
		t.Fatalf("%d First Orcs before the event, want 0", n)
	}
	if got := w.send(t, encodeBypass("Quest "+q232Name+" 30649-07.htm")); !shows(got, "30649-07.htm") {
		t.Fatalf("the event: got %q, want 30649-07.htm", got)
	}
	orc := w.calledFolk(t, "orc", q232FirstOrc, q232FirstOrcAt)
	if h := script.NPCOf(orc).SpawnLocation().Heading; h != q232FirstOrcHeading {
		t.Fatalf("the First Orc has heading %d, want %d", h, q232FirstOrcHeading)
	}

	// Another click while it stands calls no second one.
	w.send(t, encodeBypass("Quest "+q232Name+" 30649-07.htm"))
	if n := len(npcsOf(w.srv, q232FirstOrc)); n != 1 {
		t.Fatalf("%d First Orcs after the second event, want 1", n)
	}

	// With no talk to it the orc stays.
	w.advance(t, time.Minute)
	if n := len(npcsOf(w.srv, q232FirstOrc)); n != 1 {
		t.Fatalf("%d First Orcs a minute on, want 1", n)
	}

	w.stepTo(t, q232FirstOrcAt)
	if got := w.talk(t, "orc"); !shows(got, "30643-01.htm") {
		t.Fatalf("the First Orc: got %q, want 30643-01.htm", got)
	}
	w.send(t, encodeBypass("Quest "+q232Name+" 30643-02.htm"))
	if rows := questRows(t, w.srv, w.player, q232Name); rows["<cond>"] != "7" {
		t.Fatalf("rows %v, want cond 7", rows)
	}
	w.advance(t, 9999*time.Millisecond)
	if n := len(npcsOf(w.srv, q232FirstOrc)); n != 1 {
		t.Fatalf("%d First Orcs 9999 ms after its page, want 1", n)
	}
	w.advance(t, 2*time.Millisecond)
	if n := len(npcsOf(w.srv, q232FirstOrc)); n != 0 {
		t.Fatalf("%d First Orcs after the timer, want 0", n)
	}

	// The spot is free again: the page at cond 7 offers the event, which calls a new one.
	w.home(t)
	if got := w.talk(t, "martankus"); !shows(got, "30649-08.htm") {
		t.Fatalf("Martankus: got %q, want 30649-08.htm", got)
	}
	w.send(t, encodeBypass("Quest "+q232Name+" 30649-07.htm"))
	if n := len(calledAt(w, q232FirstOrc, q232FirstOrcAt)); n != 1 {
		t.Fatalf("%d First Orcs called after the leave, want 1", n)
	}
}
