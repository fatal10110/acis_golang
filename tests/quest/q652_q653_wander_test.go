package quest

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q652"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q653"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// An Aged Ex-Adventurer (Q652_AnAgedExAdventurer.onTimer) and Wild Maiden
// (Q653_WildMaiden.onTimer): three seconds after the quest is taken the NPC
// that gave it is deleted and a new one stands at another of the quest's
// places, never the place the last one stood at, whatever character took the
// quest. A scenario cannot show a script-spawned NPC appearing nor act on
// one (#4020 item 5), so these Boot tests play the timers; the scenarios
// play the dialogs, items and rewards.

type wanderCase struct {
	name      string
	path      string
	quest     func() script.Script
	level     int
	giver     int32 // the NPC that moves
	finisher  int32 // the NPC that ends the quest
	holds     map[int32]int32
	accept    string
	offer     string // the giver's page that offers the quest
	finish    string // the finisher's page
	finishRnd []int  // the draws of the finisher's talk
	// The places the giver moves to, by their draw, and the draws of each
	// of the two timers: the first draw equals the place the giver stands
	// at in the state of the quest (0 for the first, the last place for the
	// second), which draws again.
	places  map[int]script.Loc
	bound   int
	first   []int
	second  []int
	firstAt int
	nextAt  int
}

var wanderCases = []wanderCase{
	{
		name: "Q652_AnAgedExAdventurer", path: "quest.Q652_AnAgedExAdventurer", quest: q652.New,
		level: 46, giver: 32012, finisher: 30180,
		holds:  map[int32]int32{1464: 200},
		accept: "32012-02.htm", offer: "32012-01.htm", finish: "30180-02.htm", finishRnd: []int{99},
		places: map[int]script.Loc{
			1: {X: 79890, Y: -6132, Z: -2922, Heading: 0},
			3: {X: 94500, Y: -10129, Z: -3290, Heading: 0},
		},
		bound: 5, first: []int{0, 3}, second: []int{3, 3, 1}, firstAt: 3, nextAt: 1,
	},
	{
		name: "Q653_WildMaiden", path: "quest.Q653_WildMaiden", quest: q653.New,
		level: 36, giver: 32013, finisher: 30181,
		holds:  map[int32]int32{736: 2},
		accept: "32013-03.htm", offer: "32013-02.htm", finish: "30181-01.htm",
		places: map[int]script.Loc{
			2: {X: 71809, Y: 67377, Z: -3675, Heading: 29130},
			3: {X: 69166, Y: 88825, Z: -3447, Heading: 43886},
		},
		bound: 4, first: []int{0, 2}, second: []int{2, 2, 3}, firstAt: 2, nextAt: 3,
	},
}

func TestWanderingQuestGiversMoveThreeSecondsAfterTheQuestIsTaken(t *testing.T) {
	t.Parallel()
	for _, tc := range wanderCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			draws := &scriptDraws{}
			var items []int32
			for id := range tc.holds {
				items = append(items, id)
			}
			items = append(items, 956)
			templates := []*npc.Template{
				gameservertest.FolkTemplate("Folk", int(tc.giver)), gameservertest.FolkTemplate("Folk", int(tc.finisher)),
			}
			kinds := map[int32]script.NPCKind{tc.giver: script.KindFolk, tc.finisher: script.KindFolk}
			w := bootSeamQuest(t, tc.level, chatPages(shippedQuestPages(t, tc.name), int(tc.giver), int(tc.finisher)),
				func(srv *gameservertest.Server, objID int32) {
					for id, n := range tc.holds {
						srv.GiveItem(t, objID, id, n)
					}
				},
				x1Items(items),
				gameservertest.WithNPCs(npc.NewTable(templates)),
				gameservertest.WithNpcSpawns(nil),
				gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: tc.path}}, script.Catalog{tc.path: tc.quest}),
				gameservertest.WithScriptRand(draws.rand),
			)
			w.roles[w.player] = "player"

			// The first timer: the giver stands at the first place, so the
			// first draw is drawn again.
			giver := w.spawnFolk(t, "giver", int(tc.giver), 40, 0, 0)
			wantLines(t, "offer", w.talk(t, "giver"), w.page(t, "giver", tc.name, tc.offer))
			draws.queue(tc.first...)
			w.send(t, encodeBypass("Quest "+tc.name+" "+tc.accept))
			if got := questRows(t, w.srv, w.player, tc.name)["<cond>"]; got != "1" {
				t.Fatalf("cond = %q, want 1", got)
			}
			w.advance(t, 2900*time.Millisecond)
			wantAt(t, "before the timer", npcsOf(w.srv, tc.giver), giver.ObjectID())
			w.advance(t, 200*time.Millisecond)
			if got, want := draws.taken(), []int{tc.bound, tc.bound}; !slices.Equal(got, want) {
				t.Fatalf("draw bounds = %v, want %v", got, want)
			}
			moved := npcsOf(w.srv, tc.giver)
			if len(moved) != 1 || moved[0].ObjectID() == giver.ObjectID() {
				t.Fatalf("after the timer %d NPCs %d (the first %v), want one new", len(moved), tc.giver, moved)
			}
			wantPlace(t, "first place", moved[0], tc.places[tc.firstAt])
			w.advance(t, time.Hour)
			if left := npcsOf(w.srv, tc.giver); len(left) != 1 || left[0].ObjectID() != moved[0].ObjectID() {
				t.Fatalf("an hour on the NPC is %v, want it left standing", left)
			}

			// The place is the quest's, not the character's: the quest ends,
			// is taken again at a giver beside the character, and the last
			// place is drawn again.
			w.spawnFolk(t, "finisher", int(tc.finisher), 50, 0, 0)
			draws.queue(tc.finishRnd...)
			wantLines(t, "finish", w.talk(t, "finisher"), w.page(t, "finisher", tc.name, tc.finish))
			if rows := questRows(t, w.srv, w.player, tc.name); len(rows) != 0 {
				t.Fatalf("rows after the finish = %v, want none", rows)
			}
			draws.taken()
			second := w.spawnFolk(t, "second", int(tc.giver), 60, 0, 0)
			wantLines(t, "second offer", w.talk(t, "second"), w.page(t, "second", tc.name, tc.offer))
			draws.queue(tc.second...)
			w.send(t, encodeBypass("Quest "+tc.name+" "+tc.accept))
			w.advance(t, 3*time.Second)
			if got, want := draws.taken(), []int{tc.bound, tc.bound, tc.bound}; !slices.Equal(got, want) {
				t.Fatalf("second draw bounds = %v, want %v", got, want)
			}
			got := npcsOf(w.srv, tc.giver)
			if len(got) != 2 {
				t.Fatalf("after the second timer %d NPCs %d, want 2", len(got), tc.giver)
			}
			for _, n := range got {
				if n.ObjectID() == second.ObjectID() {
					t.Fatalf("the second giver stays after its timer")
				}
			}
			at := map[script.Loc]bool{}
			for _, n := range got {
				at[placeOf(n)] = true
			}
			for _, i := range []int{tc.firstAt, tc.nextAt} {
				if !at[script.Loc{X: tc.places[i].X, Y: tc.places[i].Y, Heading: tc.places[i].Heading}] {
					t.Fatalf("no giver at place %d %+v, got %v", i, tc.places[i], at)
				}
			}
		})
	}
}

// placeOf is where the NPC spawned: the coordinates and the heading, the
// height left to the ground.
func placeOf(n *script.NPC) script.Loc {
	l := n.SpawnLocation()
	return script.Loc{X: l.X, Y: l.Y, Heading: l.Heading}
}

func wantPlace(t *testing.T, step string, n *script.NPC, want script.Loc) {
	t.Helper()
	if got := placeOf(n); got != (script.Loc{X: want.X, Y: want.Y, Heading: want.Heading}) {
		t.Fatalf("%s: spawned at %+v, want %+v", step, got, want)
	}
}

func wantAt(t *testing.T, step string, got []*script.NPC, obj int32) {
	t.Helper()
	if len(got) != 1 || got[0].ObjectID() != obj {
		t.Fatalf("%s: NPCs %v, want the one %d", step, got, obj)
	}
}
