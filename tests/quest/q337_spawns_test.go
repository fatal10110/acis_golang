package quest

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q337"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The monsters Audience with the Land Dragon calls out as others die or are
// hit (Q337_AudienceWithTheLandDragon onMyDying and onAttacked): the
// sacrifices of the Blood Queen, the zealots of the Harit shamans, the
// guardians of the abyssal jewels and the third jewel of the cave monsters.
// A scenario cannot show a script-spawned NPC appearing nor hating the
// player, nor act on one (#4020 item 5), and its fixture monster has other
// hit points in the reference probe and in the run, so the thresholds of the
// jewels' health cannot be shared between them: these Boot tests pin the
// calls and the fragments; the scenarios play the dialogs and the drops.
const (
	q337Name = "Q337_AudienceWithTheLandDragon"
	q337Path = "quest.Q337_AudienceWithTheLandDragon"

	q337BloodQueen = 18001
	q337Sacrifice  = 27171
	q337Shaman     = 20644
	q337Matriarch  = 20645
	q337Zealot     = 27172
	q337Jewel1     = 27165
	q337Jewel2     = 27166
	q337Jewel3     = 27167
	q337Mara       = 27168
	q337Musfel     = 27169
	q337Python     = 27170

	q337Remains    = 3857
	q337Totem      = 3858
	q337Fragment1  = 3859
	q337Fragment2  = 3860
	q337Fragment3  = 3861
	q337ItemGetLog = "ItemSound.quest_itemget"
)

// q337World boots a character of level 50 whose quest is STARTED at cond with
// the quest variables rows, holding holds. The script random answers draws.
func q337World(t *testing.T, draws *scriptDraws, cond int, rows map[string]string, holds map[int32]int32) *dialogWorld {
	t.Helper()
	quest := q337.New()
	var templates []*npc.Template
	kinds := map[int32]script.NPCKind{}
	for _, id := range quest.Bind[script.EventTalked] {
		templates = append(templates, gameservertest.FolkTemplate("Folk", int(id)))
		kinds[id] = script.KindFolk
	}
	monsters := slices.Concat(quest.Bind[script.EventMyDying], quest.Bind[script.EventAttacked],
		[]int32{q337Sacrifice, q337Zealot, q337Mara, q337Musfel, q337Python})
	for _, id := range monsters {
		if _, ok := kinds[id]; ok {
			continue
		}
		templates = append(templates, proofMonster(int(id)))
		kinds[id] = script.KindHostile
	}
	return bootSeamQuest(t, 50, nil, func(srv *gameservertest.Server, objID int32) {
		insertJournal(t, srv.DB,
			journalRow{objID, q337Name, "<state>", val("STARTED")},
			journalRow{objID, q337Name, "<cond>", val(strconv.Itoa(cond))})
		for k, v := range rows {
			insertJournal(t, srv.DB, journalRow{objID, q337Name, k, val(v)})
		}
		for id, count := range holds {
			srv.GiveItem(t, objID, id, count)
		}
	},
		x1Items(quest.Items),
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: q337Path}}, script.Catalog{q337Path: q337.New}),
		gameservertest.WithScriptRand(draws.rand),
	)
}

// q337Kill kills a fresh monster of npc id beside the character and returns
// where it stood.
func (w *dialogWorld) q337Kill(t *testing.T, id int32) location.Location {
	t.Helper()
	at := location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z}
	h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(int(id)), at)
	killOnPlayerQueue(t, w.srv, w.player, h)
	w.render(t, w.srv.ReadQueued(t, w.srv.Client))
	return at
}

// q337Within checks that exactly count NPCs of id stand within 100 of
// (x, y), the point the script asked for: the call may move each of them as
// far again where geodata lets it. When hates, each is set on the character
// with the most hate.
func q337Within(t *testing.T, w *dialogWorld, id int32, count int, x, y int, hates bool) {
	t.Helper()
	called := npcsOf(w.srv, id)
	if len(called) != count {
		t.Fatalf("%d NPCs %d stand, want %d", len(called), id, count)
	}
	for _, n := range called {
		if loc := n.SpawnLocation(); loc.X < x-100 || loc.X > x+100 || loc.Y < y-100 || loc.Y > y+100 {
			t.Fatalf("NPC %d stands at %+v, want within 100 of %d,%d", id, loc, x, y)
		}
		if h := n.MostHated(); hates && (h == nil || h.ObjectID() != w.player) {
			t.Fatalf("NPC %d hates %v most, want the player", id, h)
		}
	}
}

// q337Gone checks that count NPCs of id are there 55 s after the call and
// none is left 10 s later.
func q337Gone(t *testing.T, w *dialogWorld, id int32, count int) {
	t.Helper()
	w.advance(t, 55*time.Second)
	if n := len(npcsOf(w.srv, id)); n != count {
		t.Fatalf("%d NPCs %d 55 s after the call, want %d", n, id, count)
	}
	w.advance(t, 10*time.Second)
	if n := len(npcsOf(w.srv, id)); n != 0 {
		t.Fatalf("%d NPCs %d after the minute is over, want 0", n, id)
	}
}

// q337Offsets queues the draws of count calls within spread: the first axis
// of each at the top of its range, the second at the bottom, so the call
// stands spread east and spread south of where the monster died.
func q337Offsets(draws *scriptDraws, count, spread int) {
	for range count {
		draws.queue(2*spread, 0)
	}
}

// q337Bounds checks the bounds of the draws taken since the last call.
func q337Bounds(t *testing.T, draws *scriptDraws, want ...int) {
	t.Helper()
	if got := draws.taken(); !slices.Equal(got, want) {
		t.Fatalf("draws of bounds %v, want %v", got, want)
	}
}

// TestQ337BloodQueenCallsEightSacrifices: at the first stage, Orven's
// request open and no remains held, the Blood Queen's death calls eight
// sacrifices within 100 of her, each moved east and south by the script's
// draws; any other state draws and calls nothing. They are gone after a
// minute.
func TestQ337BloodQueenCallsEightSacrifices(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		cond  int
		rows  map[string]string
		holds map[int32]int32
		want  int
	}{
		{"request open", 1, map[string]string{"drop1": "1"}, nil, 8},
		{"remains held", 1, map[string]string{"drop1": "1"}, map[int32]int32{q337Remains: 1}, 0},
		{"request closed", 1, nil, nil, 0},
		{"second stage", 2, map[string]string{"drop1": "1"}, nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			draws := &scriptDraws{}
			w := q337World(t, draws, tc.cond, tc.rows, tc.holds)
			q337Offsets(draws, 8, 100)
			at := w.q337Kill(t, q337BloodQueen)
			q337Within(t, w, q337Sacrifice, tc.want, at.X+100, at.Y-100, false)
			if tc.want == 0 {
				q337Bounds(t, draws)
				return
			}
			bounds := slices.Repeat([]int{201}, 16)
			q337Bounds(t, draws, bounds...)
			q337Gone(t, w, q337Sacrifice, tc.want)
		})
	}
}

// TestQ337HaritLizardmenCallZealots: at the first stage, Kendra's request
// open and no totem held, a Harit shaman or matriarch killed on a draw of 0
// of 5 calls three zealots within 50 of it; the draw comes before the
// request and the totem are looked at, and a quest past the first stage draws
// nothing. They are gone after a minute.
func TestQ337HaritLizardmenCallZealots(t *testing.T) {
	t.Parallel()
	for _, killed := range []int32{q337Shaman, q337Matriarch} {
		for _, tc := range []struct {
			name  string
			cond  int
			rows  map[string]string
			holds map[int32]int32
			draw  int
			want  int
			drawn bool
		}{
			{"request open", 1, map[string]string{"drop2": "1"}, nil, 0, 3, true},
			{"a draw of 1", 1, map[string]string{"drop2": "1"}, nil, 1, 0, true},
			{"request closed", 1, nil, nil, 0, 0, true},
			{"totem held", 1, map[string]string{"drop2": "1"}, map[int32]int32{q337Totem: 1}, 0, 0, true},
			{"second stage", 2, map[string]string{"drop2": "1"}, nil, 0, 0, false},
		} {
			t.Run(strconv.Itoa(int(killed))+" "+tc.name, func(t *testing.T) {
				t.Parallel()
				draws := &scriptDraws{}
				w := q337World(t, draws, tc.cond, tc.rows, tc.holds)
				draws.queue(tc.draw)
				q337Offsets(draws, 3, 50)
				at := w.q337Kill(t, killed)
				q337Within(t, w, q337Zealot, tc.want, at.X+50, at.Y-50, false)
				switch {
				case tc.want > 0:
					q337Bounds(t, draws, 5, 101, 101, 101, 101, 101, 101)
					q337Gone(t, w, q337Zealot, tc.want)
				case tc.drawn:
					q337Bounds(t, draws, 5)
				default:
					q337Bounds(t, draws)
				}
			})
		}
	}
}

// TestQ337CaveMonstersCallTheThirdJewel: at the fourth stage with no third
// fragment held, a cave maiden or keeper killed on a draw of 0 of 5 calls the
// third abyssal jewel within 50 of it; a draw of 1, the fragment held or
// another stage calls none. It is gone after a minute.
func TestQ337CaveMonstersCallTheThirdJewel(t *testing.T) {
	t.Parallel()
	for _, killed := range []int32{20134, 20287, 20246, 20277} {
		for _, tc := range []struct {
			name  string
			cond  int
			holds map[int32]int32
			draw  int
			want  int
			drawn bool
		}{
			{"no fragment", 4, nil, 0, 1, true},
			{"a draw of 1", 4, nil, 1, 0, true},
			{"fragment held", 4, map[int32]int32{q337Fragment3: 1}, 0, 0, true},
			{"third stage", 3, nil, 0, 0, false},
		} {
			t.Run(strconv.Itoa(int(killed))+" "+tc.name, func(t *testing.T) {
				t.Parallel()
				draws := &scriptDraws{}
				w := q337World(t, draws, tc.cond, map[string]string{"drop7": "1"}, tc.holds)
				draws.queue(tc.draw)
				q337Offsets(draws, 1, 50)
				at := w.q337Kill(t, killed)
				q337Within(t, w, q337Jewel3, tc.want, at.X+50, at.Y-50, false)
				switch {
				case tc.want > 0:
					q337Bounds(t, draws, 5, 101, 101)
					q337Gone(t, w, q337Jewel3, tc.want)
				case tc.drawn:
					q337Bounds(t, draws, 5)
				default:
					q337Bounds(t, draws)
				}
			})
		}
	}
}

// jewelFight is a world with one abyssal jewel to hit.
type jewelFight struct {
	w     *dialogWorld
	draws *scriptDraws
	jewel *npc.Hostile
	at    location.Location
}

// q337Jewel boots the world at cond with the variable rows and spawns the
// jewel id beside the character.
func q337Jewel(t *testing.T, id int32, cond int, rows map[string]string, holds map[int32]int32) *jewelFight {
	t.Helper()
	draws := &scriptDraws{}
	w := q337World(t, draws, cond, rows, holds)
	at := location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z}
	return &jewelFight{w: w, draws: draws, jewel: w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(int(id)), at), at: at}
}

// hitAt lands a hit of one on the jewel standing at percent of its health
// and returns what the character was sent.
func (f *jewelFight) hitAt(t *testing.T, percent int) []string {
	t.Helper()
	onPlayerQueue(t, f.w.srv, f.w.player, func() { f.jewel.SetCurrentHP(f.jewel.MaxHP() * percent / 100) })
	hit(t, f.w.srv, f.w.player, f.jewel, 1)
	return f.w.advance(t, 500*time.Millisecond)
}

// callDraws queues the draws of a call of count guardians: the draw that
// passes the chance, then each guardian's offsets, east and south by 150.
func (f *jewelFight) callDraws(chance, count int) {
	f.draws.queue(chance)
	q337Offsets(f.draws, count, 150)
}

// TestQ337JewelsCallGuardians: a jewel hit while its health, with the
// damage added back, is between a third and two thirds of it calls its
// guardians on a draw under 33 of 100 once the request is open, but only
// after a hit found it above 90%; the call hates the player who hit, and is
// not made again until a hit finds the jewel above 90% once more.
func TestQ337JewelsCallGuardians(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		jewel    int32
		guardian int32
		cond     int
		variable string
		count    int
	}{
		{"first jewel", q337Jewel1, q337Mara, 2, "drop5", 20},
		{"second jewel", q337Jewel2, q337Musfel, 2, "drop6", 20},
		{"third jewel", q337Jewel3, q337Python, 4, "drop7", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := q337Jewel(t, tc.jewel, tc.cond, map[string]string{tc.variable: "1"}, nil)
			calls := slices.Repeat([]int{301}, 2*tc.count)

			// The jewel has not been seen above 90% yet: a hit in the middle draws
			// and calls nothing.
			f.draws.queue(0)
			f.hitAt(t, 45)
			q337Bounds(t, f.draws, 100)
			if n := len(npcsOf(f.w.srv, tc.guardian)); n != 0 {
				t.Fatalf("%d guardians before the jewel was seen healthy, want 0", n)
			}

			// A hit that finds it above 90% draws nothing and opens the call.
			f.hitAt(t, 100)
			q337Bounds(t, f.draws)

			// A draw of 33 calls none and leaves the call open.
			f.draws.queue(33)
			f.hitAt(t, 45)
			q337Bounds(t, f.draws, 100)
			if n := len(npcsOf(f.w.srv, tc.guardian)); n != 0 {
				t.Fatalf("%d guardians on a draw of 33, want 0", n)
			}

			// A draw of 32 calls them around the jewel, each against the player.
			f.callDraws(32, tc.count)
			f.hitAt(t, 45)
			q337Bounds(t, f.draws, append([]int{100}, calls...)...)
			q337Within(t, f.w, tc.guardian, tc.count, f.at.X+150, f.at.Y-150, true)

			// The call is made: a later hit in the middle draws and calls no more.
			f.draws.queue(0)
			f.hitAt(t, 45)
			q337Bounds(t, f.draws, 100)
			q337Within(t, f.w, tc.guardian, tc.count, f.at.X+150, f.at.Y-150, true)

			// A hit between 66% and 90% leaves the call closed and draws nothing.
			f.hitAt(t, 75)
			q337Bounds(t, f.draws)

			// Above 90% again, the jewel calls once more.
			f.hitAt(t, 100)
			f.callDraws(0, tc.count)
			f.hitAt(t, 45)
			q337Bounds(t, f.draws, append([]int{100}, calls...)...)
			q337Within(t, f.w, tc.guardian, 2*tc.count, f.at.X+150, f.at.Y-150, true)
			q337Gone(t, f.w, tc.guardian, 2*tc.count)
		})
	}
}

// TestQ337JewelsCallNoGuardiansUnlessTheRequestIsOpen: a hit in the middle
// of a jewel's health draws, then calls nothing while the request is not open
// (the quest variable is not 1), the quest is at another stage (nothing is
// drawn then), or the jewel is the third one at the second stage.
func TestQ337JewelsCallNoGuardiansUnlessTheRequestIsOpen(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		jewel int32
		cond  int
		rows  map[string]string
		drawn bool
	}{
		{"request closed", q337Jewel1, 2, map[string]string{"drop5": "2"}, true},
		{"first stage", q337Jewel1, 1, map[string]string{"drop5": "1"}, false},
		{"third jewel at the second stage", q337Jewel3, 2, map[string]string{"drop7": "1"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := q337Jewel(t, tc.jewel, tc.cond, tc.rows, nil)
			f.hitAt(t, 100)
			f.draws.queue(0)
			f.hitAt(t, 45)
			if tc.drawn {
				q337Bounds(t, f.draws, 100)
			} else {
				q337Bounds(t, f.draws)
			}
			for _, id := range []int32{q337Mara, q337Musfel, q337Python} {
				if n := len(npcsOf(f.w.srv, id)); n != 0 {
					t.Fatalf("%d guardians %d called, want 0", n, id)
				}
			}
		})
	}
}

// TestQ337JewelsGiveTheirFragment: a jewel hit while its health, with the
// damage added back, is under a third of it gives the fragment, with the
// item sound, on a draw under 33 of 100 once the request is open and none is
// held; the draw comes before the request and the fragment are looked at.
func TestQ337JewelsGiveTheirFragment(t *testing.T) {
	t.Parallel()
	for _, jw := range []struct {
		name     string
		jewel    int32
		fragment int32
		cond     int
		variable string
	}{
		{"first jewel", q337Jewel1, q337Fragment1, 2, "drop5"},
		{"second jewel", q337Jewel2, q337Fragment2, 2, "drop6"},
		{"third jewel", q337Jewel3, q337Fragment3, 4, "drop7"},
	} {
		for _, tc := range []struct {
			name  string
			rows  map[string]string
			holds map[int32]int32
			draw  int
			want  int32
		}{
			{"draw of 32", map[string]string{jw.variable: "1"}, nil, 32, 1},
			{"draw of 33", map[string]string{jw.variable: "1"}, nil, 33, 0},
			{"request closed", map[string]string{jw.variable: "2"}, nil, 0, 0},
			{"fragment held", map[string]string{jw.variable: "1"}, map[int32]int32{jw.fragment: 1}, 0, 1},
		} {
			t.Run(jw.name+" "+tc.name, func(t *testing.T) {
				t.Parallel()
				f := q337Jewel(t, jw.jewel, jw.cond, tc.rows, tc.holds)
				f.draws.queue(tc.draw)
				sent := f.hitAt(t, 10)
				q337Bounds(t, f.draws, 100)
				if n := f.w.srv.PlayerItemCount(t, f.w.player, jw.fragment); n != int(tc.want) {
					t.Fatalf("fragments %d, want %d", n, tc.want)
				}
				heard := slices.ContainsFunc(sent, func(l string) bool { return strings.Contains(l, q337ItemGetLog) })
				if given := tc.want == 1 && len(tc.holds) == 0; heard != given {
					t.Fatalf("item sound heard %v, want %v in %q", heard, given, sent)
				}
			})
		}
	}
}
