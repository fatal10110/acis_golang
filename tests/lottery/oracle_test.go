package lottery

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/lottery"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// trace collects the lottery's announcements, database writes and the
// probe's own lines, one each, in the order they happen.
type trace struct{ lines []string }

func (tr *trace) add(format string, args ...any) {
	tr.lines = append(tr.lines, fmt.Sprintf(format, args...))
}

// instant formats ms, Unix milliseconds, as the probe prints an instant:
// UTC, milliseconds only when there are some.
func instant(ms int64) string {
	return strings.Replace(time.UnixMilli(ms).UTC().Format("2006-01-02T15:04:05.000Z"), ".000Z", "Z", 1)
}

// traceAnnouncer records each announcement as the probe prints it.
type traceAnnouncer struct{ tr *trace }

func (a traceAnnouncer) OnSale(round int32) {
	a.tr.add("ANN Lottery tickets are now available for Lucky Lottery #%d.", round)
}

func (a traceAnnouncer) SalesClosed() { a.tr.add("SM 783") }

func (a traceAnnouncer) Drawn(round, prize, winners int32) {
	if winners > 0 {
		a.tr.add("SM 1112 %d %d %d", round, prize, winners)
		return
	}
	a.tr.add("SM 1113 %d %d", round, prize)
}

// traceStore records each write, then makes it against the database.
type traceStore struct {
	*gamesql.LotteryStore
	tr *trace
}

func (s traceStore) InsertRound(ctx context.Context, id int32, endDate int64, prize int32) error {
	s.tr.add("DB insert idnr=%d enddate=%s prize=%d newprize=%d", id, instant(endDate), prize, prize)
	return s.LotteryStore.InsertRound(ctx, id, endDate, prize)
}

func (s traceStore) SavePrize(ctx context.Context, id, prize int32) error {
	s.tr.add("DB update_prize idnr=%d prize=%d newprize=%d", id, prize, prize)
	return s.LotteryStore.SavePrize(ctx, id, prize)
}

func (s traceStore) FinishRound(ctx context.Context, id, prize, newPrize int32, d lottery.Draw) error {
	s.tr.add("DB finish idnr=%d prize=%d newprize=%d number1=%d number2=%d prize1=%d prize2=%d prize3=%d",
		id, prize, newPrize, d.Low, d.High, d.Prize1, d.Prize2, d.Prize3)
	return s.LotteryStore.FinishRound(ctx, id, prize, newPrize, d)
}

// round is a stored games row.
type round struct {
	idnr, prize, newprize, finished, number1, number2, prize1, prize2, prize3 int32
	end                                                                       string
}

// ticket is a stored lottery ticket: its round and numbers.
type ticket struct{ round, low, high int32 }

// buy is a scripted ticket sale at an instant.
type buy struct {
	at        string
	price     int32
	low, high int32
}

// scenario is one probe run: the clock's start, how many instants to step
// through, the stored rounds and tickets, the drawing's scripted rolls and
// the sales made along the way.
type scenario struct {
	name     string
	start    string
	instants int
	rounds   []round
	tickets  []ticket
	buys     []buy
}

// rollsA are the scripted rolls: 2,2,6,16,17,19 draw 3, a repeated 3 rolled
// again, 7, 17, 18 and 20; the rest feed later drawings.
var rollsA = []int{2, 2, 6, 16, 17, 19, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19}

// scenarios are the probe's runs, in its order.
var scenarios = []scenario{
	{
		name: "empty table, Wednesday", start: "2026-10-07T10:00:00.123Z", instants: 7,
		tickets: []ticket{{1, 68, 11}, {1, 68, 11}, {1, 68 | 1, 3}, {1, 4 | 1 | 2, 1 | 2}, {1, 64 | 1 | 2 | 8, 4}, {1, 1 | 2 | 8 | 16 | 32, 0}, {2, 68, 11}},
		buys: []buy{
			{"2026-10-08T00:00:00Z", 2000, 1 | 2 | 4 | 8 | 16, 0},
			{"2026-10-10T18:55:00Z", 2000, 1 | 2 | 4 | 8 | 16, 0},
			{"2026-10-10T19:30:00Z", 2000, 4, 1 | 2 | 8 | 4},
		},
	},
	{
		name: "last row finished, Saturday evening", start: "2026-10-10T20:30:00Z", instants: 3,
		rounds: []round{{idnr: 4, end: "2026-10-03T19:00:00Z", prize: 90000, newprize: 70000, finished: 1, number1: 3, number2: 3}},
	},
	{
		name: "unfinished round far from its end", start: "2026-10-09T12:00:00Z", instants: 3,
		rounds:  []round{{idnr: 7, end: "2026-10-10T19:00:00.250Z", prize: 64000, newprize: 64000}},
		tickets: []ticket{{7, 68, 11}, {7, 4, 1 | 2}},
	},
	{
		name: "unfinished round five minutes from its end", start: "2026-10-10T18:55:00Z", instants: 2,
		rounds: []round{{idnr: 7, end: "2026-10-10T19:00:00Z", prize: 64000, newprize: 64000}},
	},
	{
		name: "unfinished round two minutes from its end", start: "2026-10-10T18:58:00Z", instants: 2,
		rounds: []round{{idnr: 7, end: "2026-10-10T19:00:00Z", prize: 64000, newprize: 64000}},
	},
	{
		name: "unfinished round two weeks past", start: "2026-10-04T09:00:00Z", instants: 6,
		rounds: []round{
			{idnr: 2, end: "2026-09-12T19:00:00Z", prize: 52000, newprize: 51000, finished: 1, number1: 1, number2: 1},
			{idnr: 3, end: "2026-09-19T19:00:00Z", prize: 51000, newprize: 51000},
		},
		tickets: []ticket{{3, 68, 11}, {3, 68, 1}},
	},
	{name: "empty table, Saturday morning", start: "2026-10-10T10:00:00Z", instants: 1},
	{name: "empty table, Saturday after the draw", start: "2026-10-10T19:30:00.999Z", instants: 1},
	{name: "empty table, Sunday midnight", start: "2026-10-11T00:00:00Z", instants: 1},
	{name: "empty table, week across the year", start: "2026-12-30T08:00:00.500Z", instants: 1},
	{name: "empty table, Friday late", start: "2027-01-01T23:59:59.999Z", instants: 1},
}

func parseInstant(t *testing.T, s string) time.Time {
	t.Helper()
	at, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

// ticketObjectID numbers the seeded and sold tickets' item rows.
var ticketObjectID int32 = 300000

// seedTicket stores a ticket of round with the given numbers.
func seedTicket(t *testing.T, db *sql.DB, round, low, high int32) {
	t.Helper()
	ticketObjectID++
	if _, err := db.Exec("INSERT INTO items (owner_id, object_id, item_id, count, enchant_level, loc, loc_data, custom_type1, custom_type2, mana_left, time) VALUES (?, ?, ?, 1, ?, 'INVENTORY', 0, ?, ?, -1, 0)",
		100, ticketObjectID, lottery.TicketID, low, round, high); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
}

// runScenario runs sc on its own database and returns its trace.
func runScenario(t *testing.T, sc scenario) []string {
	t.Helper()
	db := sqltest.SharedDB(t)
	for _, r := range sc.rounds {
		if _, err := db.Exec("INSERT INTO games (id, idnr, number1, number2, prize, newprize, prize1, prize2, prize3, enddate, finished) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			r.idnr, r.number1, r.number2, r.prize, r.newprize, r.prize1, r.prize2, r.prize3, parseInstant(t, r.end).UnixMilli(), r.finished); err != nil {
			t.Fatalf("seed round: %v", err)
		}
	}
	for _, tk := range sc.tickets {
		seedTicket(t, db, tk.round, tk.low, tk.high)
	}
	rolls := append([]int(nil), rollsA...)
	roll := func(n int) int {
		if n != 20 || len(rolls) == 0 {
			t.Fatalf("roll(%d) with %d scripted rolls left", n, len(rolls))
		}
		r := rolls[0]
		rolls = rolls[1:]
		return r
	}

	tr := &trace{lines: []string{"# " + sc.name}}
	loop := sim.NewInline(parseInstant(t, sc.start))
	l := lottery.New(lottery.DefaultConfig(), traceStore{gamesql.NewLotteryStore(db), tr}, nil, traceAnnouncer{tr}, loop.NewQueue("lottery"), zerolog.Nop(), lottery.WithRoll(roll))
	if err := l.Restore(context.Background()); err != nil {
		t.Fatalf("restore: %v", err)
	}
	l.Start()
	loop.Run()
	state := func() {
		st := l.Status()
		tr.add("@%s round=%d prize=%d started=%t selling=%t end=%s enddate=%s",
			instant(loop.Now().UnixMilli()), st.Round, st.Prize, st.Started, st.Selling, instant(st.EndDate), l.FormatDate(st.EndDate))
	}
	state()
	buys := sc.buys
	for range sc.instants {
		next, ok := loop.NextTimer()
		if !ok {
			break
		}
		for len(buys) > 0 && parseInstant(t, buys[0].at).Before(next) {
			at := parseInstant(t, buys[0].at)
			if at.After(loop.Now()) {
				loop.Advance(at.Sub(loop.Now()))
			}
			if st := l.Status(); st.Started && st.Selling {
				l.IncreasePrize(buys[0].price)
				seedTicket(t, db, st.Round, buys[0].low, buys[0].high)
				tr.add("BUY round=%d enchant=%d type2=%d", st.Round, buys[0].low, buys[0].high)
			} else {
				tr.add("BUY refused started=%t selling=%t", st.Started, st.Selling)
			}
			buys = buys[1:]
		}
		loop.Advance(max(next.Sub(loop.Now()), 0))
		state()
	}
	l.Stop()
	return tr.lines
}

// formatInts writes nums as the probe's Arrays.toString does.
func formatInts(nums []int) string {
	parts := make([]string, len(nums))
	for i, n := range nums {
		parts[i] = strconv.Itoa(n)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// tables are the probe's pure-function sections: ticket decoding, ticket
// checks, the payout, the instructions page's shares, the drawing date and
// the ticket form's buttons.
func tables(t *testing.T) []string {
	t.Helper()
	var out []string
	add := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }

	add("# decodeNumbers")
	for _, d := range [][2]int32{{0, 0}, {68, 11}, {1 | 2 | 4 | 8 | 16, 0}, {0, 15}, {32768 | 1, 1 | 2 | 4}} {
		nums := lottery.Decode(lottery.Numbers{Low: d[0], High: d[1]})
		add("decode %d %d = %s", d[0], d[1], formatInts(nums[:]))
	}

	add("# checkTicket")
	db := sqltest.SharedDB(t)
	for _, stmt := range []string{
		"INSERT INTO games (id, idnr, number1, number2, prize1, prize2, prize3, finished) VALUES (1, 9, 68, 11, 30000, 7000, 3000, 1)",
		"INSERT INTO games (id, idnr) VALUES (1, 10)",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed rounds: %v", err)
		}
	}
	loop := sim.NewInline(time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC))
	l := lottery.New(lottery.DefaultConfig(), gamesql.NewLotteryStore(db), nil, traceAnnouncer{&trace{}}, loop.NewQueue("lottery"), zerolog.Nop())
	if err := l.Restore(context.Background()); err != nil {
		t.Fatalf("restore: %v", err)
	}
	for _, c := range [][3]int32{{9, 68, 11}, {9, 68 | 1, 3}, {9, 4 | 1 | 2, 1 | 2}, {9, 4 | 64 | 1 | 2 | 8, 0}, {9, 4 | 1 | 2 | 8 | 16, 0}, {9, 1 | 2 | 8 | 16 | 32, 0}, {9, 0, 0}, {10, 68, 11}, {11, 68, 11}, {9, -1, -1}, {9, 68 | 0x10000, 11 | 0x10000}} {
		place, prize := l.Check(c[0], lottery.Numbers{Low: c[1], High: c[2]})
		add("check %d %d %d = [%d, %d]", c[0], c[1], c[2], place, prize)
	}

	add("# prizes")
	cfg := lottery.DefaultConfig()
	for _, c := range [][5]int32{{50000, 1, 0, 0, 0}, {100000, 2, 3, 4, 5}, {64000, 0, 1, 0, 2}, {1000, 1, 1, 1, 10}, {2147483000, 1, 1, 1, 0}, {-2147483000, 1, 1, 1, 1}, {50000, 0, 0, 0, 0}, {50000, 3, 0, 0, 0}} {
		p1, p2, p3, next := cfg.Payout(c[0], [4]int32{c[1], c[2], c[3], c[4]})
		add("prizes %d %d %d %d %d = %d %d %d %d", c[0], c[1], c[2], c[3], c[4], p1, p2, p3, next)
	}

	add("# rates")
	for _, r := range []struct {
		java string
		rate float64
	}{{"0.6", 0.6}, {"0.2", 0.2}, {"0.15", 0.15}, {"0.1", 0.1}, {"0.3", 0.3}, {"0.7", 0.7}, {"1.0", 1}, {"0.0", 0}, {"0.333", 0.333}, {"1.0E-5", 0.00001}, {"100000.0", 100000}, {"0.07", 0.07}} {
		rated := lottery.New(lottery.Config{FiveNumberRate: r.rate}, nil, nil, nil, loop.NewQueue("rates"), zerolog.Nop())
		add("rate %s = %s", r.java, rated.Instructions("%prize5%"))
	}

	add("# enddate")
	for _, d := range []string{"2026-10-10T19:00:00Z", "2027-01-02T19:00:00.500Z", "2026-05-02T00:00:00Z", "2026-12-31T23:59:59Z"} {
		ms := parseInstant(t, d).UnixMilli()
		add("date %s = %s", instant(ms), l.FormatDate(ms))
	}

	add("# buttons")
	for _, seq := range [][]int{{5, 5}, {1, 2, 3, 4, 5, 6, 3, 7, 20, 21, 7}, {21}, {20, 19, 18, 17, 16}, {9, 10, 9}} {
		var picks lottery.Picks
		line := "press"
		for _, val := range seq {
			picks.Press(val)
			line += " " + strconv.Itoa(val) + "->" + strings.ReplaceAll(formatInts(picks[:]), " ", "")
			if _, full := picks.Numbers(); full {
				line += "!"
			}
		}
		add("%s", line)
	}
	return out
}

// TestLotteryMatchesReferenceProbe runs the lottery on a virtual clock
// against the database, in UTC, and compares every announcement, database
// write and the state after each instant, then its ticket, payout and page
// helpers, with testdata/lottery.golden.
//
// The golden file is the output of a probe of the reference lottery (aCis
// 409, data/manager/LotteryManager.java: StartLottery, StopSellingTickets,
// FinishLottery, increasePrize, decodeNumbers, checkTicket; and
// model/actor/Npc.java showLotoWindow's form buttons, instructions shares
// and drawing date), copied with System.currentTimeMillis read from a
// virtual clock, ThreadPool.schedule replaced by stepping that clock to the
// soonest task, Rnd.get(20) by scripted rolls, and the games table, the
// ticket items and the broadcasts by trace lines. The probe and how to
// regenerate the golden file are in testdata/oracle.
//
// The runs cover an empty table on each kind of day (Wednesday, Saturday
// before and after the drawing hour, Sunday, a week across the year end),
// a finished last round, an unfinished one far from, five minutes from and
// two minutes from its drawing, and one two weeks past, which draws the
// missed rounds one a minute until a round's drawing lies ahead; the first
// run also sells tickets before and after sales close and wins every
// place.
func TestLotteryMatchesReferenceProbe(t *testing.T) {
	want, err := os.ReadFile("testdata/lottery.golden")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { got = append(got, runScenario(t, sc)...) })
	}
	t.Run("tables", func(t *testing.T) { got = append(got, tables(t)...) })
	if g, w := strings.Join(got, "\n")+"\n", string(want); g != w {
		gl, wl := strings.Split(g, "\n"), strings.Split(w, "\n")
		for i := range min(len(gl), len(wl)) {
			if gl[i] != wl[i] {
				t.Fatalf("line %d = %q, want %q", i+1, gl[i], wl[i])
			}
		}
		t.Fatalf("trace has %d lines, want %d", len(gl), len(wl))
	}
}
