package olympiad

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
)

// Two nobles of the match tests; side 1 is always drawn first.
const (
	nobleOne int32 = 1001
	nobleTwo int32 = 1002
)

// gameStart and gameEnd bound every judged fight: 90 seconds.
var (
	gameStart = time.UnixMilli(1790000000000)
	gameEnd   = gameStart.Add(90 * time.Second)
)

// onlineRoster finds every object id online, named as seedNobles names it,
// with base class 88, except those listed offline.
func onlineRoster(offline ...int32) olympiad.Roster {
	return func(id int32) (olympiad.Competitor, bool) {
		if slices.Contains(offline, id) {
			return olympiad.Competitor{}, false
		}
		return olympiad.Competitor{Name: fmt.Sprintf("Noble%d", id), BaseClass: 88}, true
	}
}

// first always draws the first queued registration.
func first(int) int { return 0 }

// fightRow is one olympiad_fights row.
type fightRow struct {
	one, two, oneClass, twoClass, winner int
	start, length                        int64
	classed                              int
}

func fightRows(t *testing.T, db *sql.DB) []fightRow {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), "SELECT charOneId, charTwoId, charOneClass, charTwoClass, winner, start, time, classed FROM olympiad_fights")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []fightRow
	for rows.Next() {
		var r fightRow
		if err := rows.Scan(&r.one, &r.two, &r.oneClass, &r.twoClass, &r.winner, &r.start, &r.length, &r.classed); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func won(name string) olympiad.ResultMessage {
	return olympiad.ResultMessage{Kind: olympiad.ResultWon, Name: name}
}

func tie() olympiad.ResultMessage { return olympiad.ResultMessage{Kind: olympiad.ResultTie} }

func gained(name string, n int) olympiad.ResultMessage {
	return olympiad.ResultMessage{Kind: olympiad.ResultPointsGained, Name: name, Points: n}
}

func lost(name string, n int) olympiad.ResultMessage {
	return olympiad.ResultMessage{Kind: olympiad.ResultPointsLost, Name: name, Points: n}
}

// record is the part of a noble's record a match changes.
type record struct{ points, done, won, lost, drawn int }

// TestOlympiadGameJudging pins how a one-on-one match is judged, with the
// shipped settings (at most 10 points a match, dividers 3 classed and 5
// non-classed, rewards 50 and 30 Noblesse Gate Passes). The expected
// values follow the reference's validateWinner by hand: a decided match
// moves min(points)/divider points clamped to [1, 10], a tie costs each
// side its own points/divider capped at 10, a defection a third of the
// defector's points capped at 10.
func TestOlympiadGameJudging(t *testing.T) {
	t.Parallel()
	alive := olympiad.Standing{Online: true, HP: 1000, CP: 500}
	dead := olympiad.Standing{Online: true, Dead: true}
	offline := olympiad.Standing{}
	gate := func(n int) []olympiad.Reward { return []olympiad.Reward{{ItemID: 6651, Count: n}} }
	for _, tc := range []struct {
		name               string
		kind               olympiad.GameType
		onePoints          int
		twoPoints          int
		oneStand, twoStand olympiad.Standing
		damage             [2]int
		disconnect         []int32
		defect             []int32
		want               []olympiad.ResultMessage
		winner             int32
		reward             []olympiad.Reward
		oneRec, twoRec     record
		fights             []fightRow
	}{
		{
			name: "side two dies", kind: olympiad.NonClassed, onePoints: 50, twoPoints: 30,
			oneStand: alive, twoStand: dead,
			// 30/5 = 6.
			want:   []olympiad.ResultMessage{won("Noble1001"), gained("Noble1001", 6), lost("Noble1002", 6)},
			winner: nobleOne, reward: gate(30),
			oneRec: record{points: 56, done: 1, won: 1}, twoRec: record{points: 24, done: 1, lost: 1},
			fights: []fightRow{{1001, 1002, 88, 88, 1, gameStart.UnixMilli(), 90000, 0}},
		},
		{
			name: "classed, side one dies", kind: olympiad.Classed, onePoints: 20, twoPoints: 20,
			oneStand: dead, twoStand: alive,
			// 20/3 = 6.
			want:   []olympiad.ResultMessage{won("Noble1002"), gained("Noble1002", 6), lost("Noble1001", 6)},
			winner: nobleTwo, reward: gate(50),
			oneRec: record{points: 14, done: 1, lost: 1}, twoRec: record{points: 26, done: 1, won: 1},
			fights: []fightRow{{1001, 1002, 88, 88, 2, gameStart.UnixMilli(), 90000, 1}},
		},
		{
			name: "more damage wins", kind: olympiad.NonClassed, onePoints: 18, twoPoints: 18,
			oneStand: alive, twoStand: alive, damage: [2]int{100, 101},
			// 18/5 = 3.
			want:   []olympiad.ResultMessage{won("Noble1002"), gained("Noble1002", 3), lost("Noble1001", 3)},
			winner: nobleTwo, reward: gate(30),
			oneRec: record{points: 15, done: 1, lost: 1}, twoRec: record{points: 21, done: 1, won: 1},
			fights: []fightRow{{1001, 1002, 88, 88, 2, gameStart.UnixMilli(), 90000, 0}},
		},
		{
			name: "under half a point left counts as down", kind: olympiad.NonClassed, onePoints: 10, twoPoints: 10,
			oneStand: olympiad.Standing{Online: true, HP: 0.3, CP: 0.1}, twoStand: alive, damage: [2]int{500, 0},
			// 10/5 = 2.
			want:   []olympiad.ResultMessage{won("Noble1002"), gained("Noble1002", 2), lost("Noble1001", 2)},
			winner: nobleTwo, reward: gate(30),
			oneRec: record{points: 8, done: 1, lost: 1}, twoRec: record{points: 12, done: 1, won: 1},
			fights: []fightRow{{1001, 1002, 88, 88, 2, gameStart.UnixMilli(), 90000, 0}},
		},
		{
			name: "even damage is a tie", kind: olympiad.NonClassed, onePoints: 70, twoPoints: 12,
			oneStand: alive, twoStand: alive, damage: [2]int{40, 40},
			// min(70/5, 10) = 10 and 12/5 = 2.
			want:   []olympiad.ResultMessage{tie(), lost("Noble1001", 10), lost("Noble1002", 2)},
			oneRec: record{points: 60, done: 1}, twoRec: record{points: 10, done: 1},
			fights: []fightRow{{1001, 1002, 88, 88, 0, gameStart.UnixMilli(), 90000, 0}},
		},
		{
			name: "side two disconnects", kind: olympiad.NonClassed, onePoints: 3, twoPoints: 40,
			oneStand: alive, twoStand: offline, disconnect: []int32{nobleTwo},
			// 3/5 = 0, raised to 1; no fight row.
			want:   []olympiad.ResultMessage{won("Noble1001"), gained("Noble1001", 1), lost("Noble1002", 1)},
			winner: nobleOne, reward: gate(30),
			oneRec: record{points: 4, done: 1, won: 1}, twoRec: record{points: 39, done: 1, lost: 1},
		},
		{
			name: "both disconnect", kind: olympiad.NonClassed, onePoints: 100, twoPoints: 100,
			oneStand: offline, twoStand: offline, disconnect: []int32{nobleOne, nobleTwo},
			// min(100/5, 10) = 10 off each.
			want:   []olympiad.ResultMessage{tie(), lost("Noble1001", 10), lost("Noble1002", 10)},
			oneRec: record{points: 90, done: 1, lost: 1}, twoRec: record{points: 90, done: 1, lost: 1},
		},
		{
			name: "both offline without a disconnect is a draw", kind: olympiad.NonClassed, onePoints: 30, twoPoints: 30,
			oneStand: offline, twoStand: offline,
			want:   []olympiad.ResultMessage{tie()},
			oneRec: record{points: 30, done: 1, drawn: 1}, twoRec: record{points: 30, done: 1, drawn: 1},
		},
		{
			name: "offline without a disconnect loses", kind: olympiad.NonClassed, onePoints: 30, twoPoints: 30,
			oneStand: olympiad.Standing{HP: 1000, CP: 500}, twoStand: alive,
			want:   []olympiad.ResultMessage{won("Noble1002"), gained("Noble1002", 6), lost("Noble1001", 6)},
			winner: nobleTwo, reward: gate(30),
			oneRec: record{points: 24, done: 1, lost: 1}, twoRec: record{points: 36, done: 1, won: 1},
			fights: []fightRow{{1001, 1002, 88, 88, 2, gameStart.UnixMilli(), 90000, 0}},
		},
		{
			name: "defection costs a third, nothing else", kind: olympiad.NonClassed, onePoints: 40, twoPoints: 9,
			oneStand: alive, twoStand: alive, defect: []int32{nobleOne, nobleTwo},
			// min(40/3, 10) = 10 and 9/3 = 3; the match is not counted.
			want:   []olympiad.ResultMessage{lost("Noble1001", 10), lost("Noble1002", 3)},
			oneRec: record{points: 30}, twoRec: record{points: 6},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			db := sqltest.SharedDB(t)
			seedNobles(t, db, 1, map[int32]int{nobleOne: tc.onePoints, nobleTwo: tc.twoPoints})
			o, _ := newOlympiad(t, gamesql.NewOlympiadStore(db), gameStart)

			queue := []int32{nobleOne, nobleTwo}
			var g *olympiad.Game
			if tc.kind == olympiad.Classed {
				ready := []*[]int32{&queue}
				g = o.ClassedGame(3, &ready, onlineRoster(), first)
			} else {
				g = o.NonClassedGame(3, &queue, onlineRoster(), first)
			}
			if g == nil {
				t.Fatal("no match drawn")
			}
			g.Start(gameStart)
			g.ResetDamage()
			g.AddDamage(nobleOne, tc.damage[0])
			g.AddDamage(nobleTwo, tc.damage[1])
			for _, id := range tc.disconnect {
				g.Disconnect(id)
			}
			for _, id := range tc.defect {
				g.Defect(id)
			}

			got := g.Judge(gameEnd, func(id int32) olympiad.Standing {
				if id == nobleOne {
					return tc.oneStand
				}
				return tc.twoStand
			})
			want := olympiad.Outcome{Messages: tc.want, Winner: tc.winner, Reward: tc.reward}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("outcome = %+v, want %+v", got, want)
			}
			for id, wantRec := range map[int32]record{nobleOne: tc.oneRec, nobleTwo: tc.twoRec} {
				n, _ := o.Noble(id)
				if gotRec := (record{n.Points, n.CompDone, n.CompWon, n.CompLost, n.CompDrawn}); gotRec != wantRec {
					t.Errorf("record %d = %+v, want %+v", id, gotRec, wantRec)
				}
			}

			o.Stop(ctx)
			if got := fightRows(t, db); !slices.Equal(got, tc.fights) {
				t.Fatalf("olympiad_fights = %+v, want %+v", got, tc.fights)
			}
			var points int
			if err := db.QueryRowContext(ctx, "SELECT olympiad_points FROM olympiad_nobles WHERE char_id = ?", nobleOne).Scan(&points); err != nil || points != tc.oneRec.points {
				t.Fatalf("stored points of %d = %d, %v; want %d", nobleOne, points, err, tc.oneRec.points)
			}
		})
	}
}

// TestOlympiadGameDraw pins how a match draws its competitors from the
// registrations: each draw takes the competitor off the queue; a first
// draw found offline is dropped; a second one found offline is dropped and
// the first goes back to the end of the queue. A queue left with fewer
// than two gives no match. A classed match picks one class queue at random
// and drops from the ready list each queue that cannot give a pair: one
// holding a single registration, untouched, or one whose first draw is
// offline, which leaves its last registration queued.
func TestOlympiadGameDraw(t *testing.T) {
	t.Parallel()
	db := sqltest.SharedDB(t)
	o, _ := newOlympiad(t, gamesql.NewOlympiadStore(db), gameStart)

	queue := []int32{1, 2, 3, 4}
	g := o.NonClassedGame(0, &queue, onlineRoster(1, 3), first)
	// 1 is dropped; 2 is drawn, 3 dropped and 2 queued behind 4; then 4
	// and 2.
	if g == nil {
		t.Fatal("no match drawn")
	}
	got := g.Participants()
	if got[0].ObjectID != 4 || got[0].Side != 1 || got[1].ObjectID != 2 || got[1].Side != 2 || got[0].Name != "Noble4" {
		t.Fatalf("participants = %+v, want 4 on side 1 and 2 on side 2", got)
	}
	if len(queue) != 0 || g.Type() != olympiad.NonClassed || g.Stadium() != 0 {
		t.Fatalf("queue %v, type %v, stadium %d; want empty, non-classed, 0", queue, g.Type(), g.Stadium())
	}
	if !g.Contains(4) || g.Contains(1) {
		t.Fatal("Contains does not follow the participants")
	}
	if g := o.NonClassedGame(0, &[]int32{5}, onlineRoster(), first); g != nil {
		t.Fatalf("a single registration drew %+v", g.Participants())
	}

	short, gone, full := []int32{10}, []int32{20, 21}, []int32{30, 31, 32}
	ready := []*[]int32{&short, &gone, &full}
	g = o.ClassedGame(5, &ready, onlineRoster(20, 21), first)
	if g == nil || g.Type() != olympiad.Classed || g.Stadium() != 5 {
		t.Fatalf("classed draw = %v, want a classed match in stadium 5", g)
	}
	if got := g.Participants(); got[0].ObjectID != 30 || got[1].ObjectID != 31 {
		t.Fatalf("classed participants = %+v, want 30 and 31", got)
	}
	if len(ready) != 1 || ready[0] != &full || !slices.Equal(gone, []int32{21}) || !slices.Equal(full, []int32{32}) || !slices.Equal(short, []int32{10}) {
		t.Fatalf("after the classed draw: ready %d queues, gone %v, full %v, short %v", len(ready), gone, full, short)
	}
}
