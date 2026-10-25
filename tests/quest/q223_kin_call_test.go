package quest

import (
	"strconv"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q223"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Test of the Champion's kin calls (Q223_TestOfTheChampion.onAttacked): at
// cond 6 a harpy hit by the player may call harpy matriarchs, at cond 10 a
// road scavenger road collectors, each on the player, once per caller. The
// scenarios cannot play the call: a scenario cannot show a script-spawned
// NPC appearing, nor act on one (#4020 item 5). These Boot tests pin it.
const (
	q223Name           = "Q223_TestOfTheChampion"
	q223Path           = "quest.Q223_TestOfTheChampion"
	q223Harpy          = 20145
	q223HarpyMatriarch = 27088
	q223RoadScavenger  = 20551
	q223RoadCollector  = 27089
)

// q223World boots a level 39 character whose Test of the Champion is at
// cond. The coin flip of the call draws flip; the draws of [0, 10) take
// tens in order, then 0.
func q223World(t *testing.T, cond, flip int, tens ...int) *dialogWorld {
	t.Helper()
	var mu sync.Mutex
	w := bootSeamQuest(t, 39, nil, func(srv *gameservertest.Server, objID int32) {
		insertJournal(t, srv.DB,
			journalRow{objID, q223Name, "<state>", val("STARTED")},
			journalRow{objID, q223Name, "<cond>", val(strconv.Itoa(cond))})
	},
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			proofMonster(q223Harpy), proofMonster(q223HarpyMatriarch),
			proofMonster(q223RoadScavenger), proofMonster(q223RoadCollector),
		})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{
				q223Harpy: script.KindHostile, q223HarpyMatriarch: script.KindHostile,
				q223RoadScavenger: script.KindHostile, q223RoadCollector: script.KindHostile,
			},
			[]script.Listing{{Path: q223Path}},
			script.Catalog{q223Path: q223.New},
		),
		gameservertest.WithScriptRand(func(n int) int {
			mu.Lock()
			defer mu.Unlock()
			switch {
			case n == 2:
				return flip
			case n == 10 && len(tens) > 0:
				v := tens[0]
				tens = tens[1:]
				return v
			}
			return 0
		}),
	)
	w.roles[w.player] = "player"
	return w
}

// TestQ223KinCalls: a hit at the calling cond with the coin flip's true (a
// draw of 0 of 2) calls one helper within 100 of the caller on each axis,
// set on the player; the loop's bound is drawn again before each spawn, so
// a second draw of 7 or more of 10 calls a second one. A flip of false, or
// another cond, calls none; a caller that called once never calls again.
func TestQ223KinCalls(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name           string
		caller, helper int32
		cond, flip     int
		tens           []int
		want           int
	}{
		{"harpy, one matriarch", q223Harpy, q223HarpyMatriarch, 6, 0, []int{9, 6}, 1},
		{"harpy, two matriarchs", q223Harpy, q223HarpyMatriarch, 6, 0, []int{0, 7, 0}, 2},
		{"harpy, flip false", q223Harpy, q223HarpyMatriarch, 6, 1, nil, 0},
		{"harpy, cond 10", q223Harpy, q223HarpyMatriarch, 10, 0, nil, 0},
		{"road scavenger, one collector", q223RoadScavenger, q223RoadCollector, 10, 0, []int{9, 6}, 1},
		{"road scavenger, two collectors", q223RoadScavenger, q223RoadCollector, 10, 0, []int{0, 9, 0}, 2},
		{"road scavenger, flip false", q223RoadScavenger, q223RoadCollector, 10, 1, nil, 0},
		{"road scavenger, cond 6", q223RoadScavenger, q223RoadCollector, 6, 0, nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := q223World(t, tc.cond, tc.flip, tc.tens...)
			h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(int(tc.caller)), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
			x, y, _ := h.Position()
			hit(t, w.srv, w.player, h, 1)
			helpers := npcsOf(w.srv, tc.helper)
			if len(helpers) != tc.want {
				t.Fatalf("%d helpers after the hit, want %d", len(helpers), tc.want)
			}
			for _, n := range helpers {
				if loc := n.SpawnLocation(); loc.X < x-100 || loc.X > x+100 || loc.Y < y-100 || loc.Y > y+100 {
					t.Fatalf("a helper stands at (%d, %d), want within 100 of (%d, %d)", loc.X, loc.Y, x, y)
				}
				if most := n.MostHated(); most == nil || most.ObjectID() != w.player {
					t.Fatalf("a helper hates %v most, want the player", most)
				}
			}
			// The caller calls once: a second hit, whatever its draws, calls no more.
			hit(t, w.srv, w.player, h, 1)
			if n := len(npcsOf(w.srv, tc.helper)); n != tc.want {
				t.Fatalf("%d helpers after a second hit, want %d", n, tc.want)
			}
		})
	}
}
