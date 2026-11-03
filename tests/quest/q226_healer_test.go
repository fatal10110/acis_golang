package quest

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q226"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Test of the Healer's called monsters (Q226_TestOfTheHealer onAdvEvent
// "30428-02.htm" and "30674-02.htm", onTalk of Daurin Hammercrush and the
// mysterious dark elf, onDecayed, onMyDying): Perrin's dialog calls one
// Tatoma and Daurin's one Leto lizardman leader, each for 200 s, the dark
// elf calls the ambushes around the player. A scenario cannot show a
// script-spawned NPC appearing or leaving, nor act on one (#4020 item 5), so
// these Boot tests pin them; the scenarios play the dialogs and the kills of
// the monsters they stand.
const (
	q226Name     = "Q226_TestOfTheHealer"
	q226Path     = "quest.Q226_TestOfTheHealer"
	q226Perrin   = 30428
	q226Darkelf  = 30661
	q226Daurin   = 30674
	q226Leader   = 27123
	q226Assassin = 27124
	q226Sniper   = 27125
	q226Wizard   = 27126
	q226Lord     = 27127
	q226Tatoma   = 27134

	q226OrderOfSorius = 2815
	q226Letter1       = 2816
	q226Letter2       = 2817
	q226Letter3       = 2818
	q226Letter4       = 2819
)

// Where the dialogs call Tatoma and the leader.
var (
	q226TatomaAt = [3]int{-93254, 147559, -2679}
	q226LeaderAt = [3]int{-97441, 106585, -3405}
)

// q226World boots a level 39 character whose Test of the Healer is at cond,
// holding one of each of hold, with Perrin, Daurin and the dark elf (the
// roles "perrin", "daurin" and "darkelf") standing by it.
func q226World(t *testing.T, cond string, hold ...int32) *dialogWorld {
	t.Helper()
	pages := chatPages(questPages(q226Name, map[string][]string{
		"30428-01.htm":  {"30428-02.htm"},
		"30428-02.htm":  nil,
		"30674-01.htm":  {"30674-02.htm"},
		"30674-02.htm":  nil,
		"30674-02a.htm": nil,
		"30674-04.htm":  nil,
		"30661-01.htm":  nil,
		"30661-02.htm":  nil,
		"30661-03.htm":  nil,
		"30661-04.htm":  nil,
	}), q226Perrin, q226Daurin, q226Darkelf)
	w := bootSeamQuest(t, 39, pages, func(srv *gameservertest.Server, objID int32) {
		insertJournal(t, srv.DB,
			journalRow{objID, q226Name, "<state>", val("STARTED")},
			journalRow{objID, q226Name, "<cond>", val(cond)})
		for _, id := range hold {
			srv.GiveItem(t, objID, id, 1)
		}
	},
		x1Items([]int32{q226OrderOfSorius, q226Letter1, q226Letter2, q226Letter3, q226Letter4}),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			proofMonster(q226Leader), proofMonster(q226Assassin), proofMonster(q226Sniper),
			proofMonster(q226Wizard), proofMonster(q226Lord), proofMonster(q226Tatoma),
			gameservertest.FolkTemplate("Folk", q226Perrin), gameservertest.FolkTemplate("Folk", q226Daurin),
			gameservertest.FolkTemplate("Folk", q226Darkelf),
		})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{
				q226Leader: script.KindHostile, q226Assassin: script.KindHostile, q226Sniper: script.KindHostile,
				q226Wizard: script.KindHostile, q226Lord: script.KindHostile, q226Tatoma: script.KindHostile,
				q226Perrin: script.KindFolk, q226Daurin: script.KindFolk, q226Darkelf: script.KindFolk,
				// The rest of the quest's talkers, so its talk bindings register.
				30473: script.KindFolk, 30327: script.KindFolk, 30424: script.KindFolk, 30658: script.KindFolk,
				30659: script.KindFolk, 30660: script.KindFolk, 30662: script.KindFolk, 30663: script.KindFolk,
				30664: script.KindFolk, 30665: script.KindFolk,
			},
			[]script.Listing{{Path: q226Path}},
			script.Catalog{q226Path: q226.New},
		),
	)
	w.roles[w.player] = "player"
	w.spawnFolk(t, "perrin", q226Perrin, 40, 0, 0)
	w.spawnFolk(t, "daurin", q226Daurin, -40, 0, 0)
	w.spawnFolk(t, "darkelf", q226Darkelf, 0, 40, 0)
	return w
}

// calledAt returns the NPCs of npc id id standing at the spot at.
func calledAt(w *dialogWorld, id int32, at [3]int) []*script.NPC {
	var out []*script.NPC
	for _, n := range npcsOf(w.srv, id) {
		if loc := n.SpawnLocation(); loc.X == at[0] && loc.Y == at[1] && abs(loc.Z-at[2]) <= 32 {
			out = append(out, n)
		}
	}
	return out
}

// talkEvent talks to the NPC role, expecting its page, and sends the
// quest's bypass event.
func (w *dialogWorld) talkEvent(t *testing.T, role, page, event string) []string {
	t.Helper()
	if got := w.talk(t, role); !shows(got, page) {
		t.Fatalf("%s: got %q, want %s", role, got, page)
	}
	return w.send(t, encodeBypass("Quest "+q226Name+" "+event))
}

// TestQ226PerrinCallsOneTatoma: Perrin's dialog sets cond 2 and calls
// Tatoma at his spot, gone 200 s later; while one stands the dialog calls no
// other, and once it has left the dialog calls a new one.
func TestQ226PerrinCallsOneTatoma(t *testing.T) {
	t.Parallel()
	w := q226World(t, "1")
	if got := w.talkEvent(t, "perrin", "30428-01.htm", "30428-02.htm"); !shows(got, "30428-02.htm") {
		t.Fatalf("Perrin's bypass: got %q, want 30428-02.htm", got)
	}
	called := calledAt(w, q226Tatoma, q226TatomaAt)
	if len(called) != 1 {
		t.Fatalf("%d Tatoma called, want 1", len(called))
	}
	// The call passes heading 0 and 200 s to stay.
	if h := called[0].SpawnLocation().Heading; h != 0 {
		t.Fatalf("Tatoma's heading is %d, want 0", h)
	}
	if rows := questRows(t, w.srv, w.player, q226Name); rows["<cond>"] != "2" {
		t.Fatalf("rows %v after the call, want cond 2", rows)
	}
	w.talkEvent(t, "perrin", "30428-01.htm", "30428-02.htm")
	if n := len(calledAt(w, q226Tatoma, q226TatomaAt)); n != 1 {
		t.Fatalf("%d Tatoma after a second call, want 1", n)
	}
	w.advance(t, 199*time.Second)
	if n := len(calledAt(w, q226Tatoma, q226TatomaAt)); n != 1 {
		t.Fatalf("%d Tatoma 199 s after the call, want 1", n)
	}
	w.advance(t, 3*time.Second)
	if n := len(calledAt(w, q226Tatoma, q226TatomaAt)); n != 0 {
		t.Fatalf("%d Tatoma after the despawn, want 0", n)
	}
	w.talkEvent(t, "perrin", "30428-01.htm", "30428-02.htm")
	if n := len(calledAt(w, q226Tatoma, q226TatomaAt)); n != 1 {
		t.Fatalf("%d Tatoma after the call that follows, want 1", n)
	}
}

// TestQ226DaurinEventCallsOneLeader: Daurin's dialog takes Sorius's order,
// sets cond 11 with the before-battle sound and calls the Leto lizardman
// leader at his spot, gone 200 s later; while one stands the dialog calls no
// other.
func TestQ226DaurinEventCallsOneLeader(t *testing.T) {
	t.Parallel()
	w := q226World(t, "10", q226OrderOfSorius)
	sent := w.talkEvent(t, "daurin", "30674-01.htm", "30674-02.htm")
	if !shows(sent, "30674-02.htm") || !slices.Contains(sent, q217BeforeBattle) {
		t.Fatalf("Daurin's bypass: got %q, want 30674-02.htm and the before-battle sound", sent)
	}
	called := calledAt(w, q226Leader, q226LeaderAt)
	if len(called) != 1 {
		t.Fatalf("%d leaders called, want 1", len(called))
	}
	if h := called[0].SpawnLocation().Heading; h != 0 {
		t.Fatalf("the leader's heading is %d, want 0", h)
	}
	if rows := questRows(t, w.srv, w.player, q226Name); rows["<cond>"] != "11" {
		t.Fatalf("rows %v after the call, want cond 11", rows)
	}
	if n := w.srv.PlayerItemCount(t, w.player, q226OrderOfSorius); n != 0 {
		t.Fatalf("orders of Sorius %d, want 0", n)
	}
	// At cond 11 his talk calls none while this one stands.
	w.talk(t, "daurin")
	if n := len(calledAt(w, q226Leader, q226LeaderAt)); n != 1 {
		t.Fatalf("%d leaders after his talk, want 1", n)
	}
	w.advance(t, 199*time.Second)
	if n := len(calledAt(w, q226Leader, q226LeaderAt)); n != 1 {
		t.Fatalf("%d leaders 199 s after the call, want 1", n)
	}
	w.advance(t, 3*time.Second)
	if n := len(calledAt(w, q226Leader, q226LeaderAt)); n != 0 {
		t.Fatalf("%d leaders after the despawn, want 0", n)
	}
}

// TestQ226DaurinTalkCallsOneLeaderThatStays: at cond 11 with no leader
// standing, Daurin's talk calls one at his spot; the call passes 200000
// where the heading goes and no despawn time, so it never leaves on its own.
func TestQ226DaurinTalkCallsOneLeaderThatStays(t *testing.T) {
	t.Parallel()
	w := q226World(t, "11")
	if got := w.talk(t, "daurin"); !shows(got, "30674-02a.htm") {
		t.Fatalf("Daurin: got %q, want 30674-02a.htm", got)
	}
	called := calledAt(w, q226Leader, q226LeaderAt)
	if len(called) != 1 {
		t.Fatalf("%d leaders called, want 1", len(called))
	}
	if h := called[0].SpawnLocation().Heading; h != 200000 {
		t.Fatalf("the leader's heading is %d, want 200000", h)
	}
	w.talk(t, "daurin")
	w.advance(t, 10*time.Minute)
	if n := len(calledAt(w, q226Leader, q226LeaderAt)); n != 1 {
		t.Fatalf("%d leaders ten minutes and a second talk on, want 1", n)
	}
	// Once it has left, a talk calls another.
	called[0].DeleteMe()
	w.talk(t, "daurin")
	again := calledAt(w, q226Leader, q226LeaderAt)
	if len(again) != 1 || again[0].ObjectID() == called[0].ObjectID() {
		t.Fatalf("leaders %v after the first left, want one new one", again)
	}
}

// TestQ226DarkelfCallsTheAmbushes: the mysterious dark elf at cond 13, 15
// and 17 sets the next cond, plays the before-battle sound and calls, up to
// 100 from the player and never leaving on their own, three assassins, three
// snipers, then two wizards and a lord; talking again at the next cond calls
// no more.
func TestQ226DarkelfCallsTheAmbushes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		cond     string
		wantCond string
		page     string
		want     map[int32]int
	}{
		{"assassins", "13", "14", "30661-01.htm", map[int32]int{q226Assassin: 3}},
		{"snipers", "15", "16", "30661-02.htm", map[int32]int{q226Sniper: 3}},
		{"wizards and a lord", "17", "18", "30661-03.htm", map[int32]int{q226Wizard: 2, q226Lord: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := q226World(t, tc.cond)
			sent := w.talk(t, "darkelf")
			if !shows(sent, tc.page) || !slices.Contains(sent, q217BeforeBattle) {
				t.Fatalf("dark elf: got %q, want %s and the before-battle sound", sent, tc.page)
			}
			if rows := questRows(t, w.srv, w.player, q226Name); rows["<cond>"] != tc.wantCond {
				t.Fatalf("rows %v, want cond %s", rows, tc.wantCond)
			}
			check := func(when string) {
				t.Helper()
				for id, n := range tc.want {
					monsters := npcsOf(w.srv, id)
					if len(monsters) != n {
						t.Fatalf("%d of %d %s, want %d", len(monsters), id, when, n)
					}
					for _, m := range monsters {
						loc := m.SpawnLocation()
						if abs(loc.X-w.at.X) > 100 || abs(loc.Y-w.at.Y) > 100 {
							t.Fatalf("%d stands at %+v, want within 100 of the player's %+v", id, loc, w.at)
						}
					}
				}
			}
			check("after the call")
			w.talk(t, "darkelf")
			check("after a second talk")
			w.advance(t, 10*time.Minute)
			check("ten minutes on")
		})
	}
}

// TestQ226CondOutsideTheAmbushesCallsNone: the dark elf at cond 19, past the
// ambushes, only sets cond 20.
func TestQ226CondOutsideTheAmbushesCallsNone(t *testing.T) {
	t.Parallel()
	w := q226World(t, "19")
	if got := w.talk(t, "darkelf"); !shows(got, "30661-04.htm") {
		t.Fatalf("dark elf: got %q, want 30661-04.htm", got)
	}
	for _, id := range []int32{q226Assassin, q226Sniper, q226Wizard, q226Lord} {
		if n := len(npcsOf(w.srv, id)); n != 0 {
			t.Fatalf("%d of %d called, want 0", n, id)
		}
	}
	if rows := questRows(t, w.srv, w.player, q226Name); rows["<cond>"] != "20" {
		t.Fatalf("rows %v, want cond 20", rows)
	}
}

// TestQ226KillsAdvanceTheTest: each monster of the test slain at its two
// conds sets the next cond and gives its secret letter (none for Tatoma);
// slain at another cond it changes nothing.
func TestQ226KillsAdvanceTheTest(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		npc      int
		cond     string
		wantCond string
		letter   int32
	}{
		{"Tatoma at cond 1", q226Tatoma, "1", "3", 0},
		{"Tatoma at cond 2", q226Tatoma, "2", "3", 0},
		{"Tatoma at cond 3", q226Tatoma, "3", "3", 0},
		{"leader at cond 10", q226Leader, "10", "12", q226Letter1},
		{"leader at cond 11", q226Leader, "11", "12", q226Letter1},
		{"leader at cond 12", q226Leader, "12", "12", 0},
		{"assassin at cond 13", q226Assassin, "13", "15", q226Letter2},
		{"assassin at cond 14", q226Assassin, "14", "15", q226Letter2},
		{"assassin at cond 15", q226Assassin, "15", "15", 0},
		{"sniper at cond 15", q226Sniper, "15", "17", q226Letter3},
		{"sniper at cond 16", q226Sniper, "16", "17", q226Letter3},
		{"sniper at cond 17", q226Sniper, "17", "17", 0},
		{"lord at cond 17", q226Lord, "17", "19", q226Letter4},
		{"lord at cond 18", q226Lord, "18", "19", q226Letter4},
		{"lord at cond 19", q226Lord, "19", "19", 0},
		{"wizard at cond 17", q226Wizard, "17", "17", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := q226World(t, tc.cond)
			h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(tc.npc), location.Location{X: w.at.X + 80, Y: w.at.Y, Z: w.at.Z})
			killOnPlayerQueue(t, w.srv, w.player, h)
			w.srv.FlushPersistence(t)
			if rows := questRows(t, w.srv, w.player, q226Name); rows["<cond>"] != tc.wantCond {
				t.Fatalf("rows %v, want cond %s", rows, tc.wantCond)
			}
			for _, letter := range []int32{q226Letter1, q226Letter2, q226Letter3, q226Letter4} {
				want := 0
				if letter == tc.letter {
					want = 1
				}
				if n := w.srv.PlayerItemCount(t, w.player, letter); n != want {
					t.Fatalf("item %d held %d, want %d", letter, n, want)
				}
			}
		})
	}
}
