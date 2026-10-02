package olympiad

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// trace collects the calendar's announcements and database writes, one
// line each, in the order they happen.
type trace struct{ lines []string }

func (tr *trace) add(format string, args ...any) {
	tr.lines = append(tr.lines, fmt.Sprintf(format, args...))
}

// traceAnnouncer records each announcement.
type traceAnnouncer struct{ tr *trace }

var noticeNames = map[olympiad.Notice]string{
	olympiad.NoticeCompetitionStarted: "game_started",
	olympiad.NoticeCompetitionEnded:   "game_ended",
	olympiad.NoticeRegistrationEnded:  "registration_ended",
	olympiad.NoticeCycleStarted:       "cycle_started",
	olympiad.NoticeCycleEnded:         "cycle_ended",
}

func (a traceAnnouncer) Announce(n olympiad.Notice, cycle int32) {
	if n == olympiad.NoticeCycleStarted || n == olympiad.NoticeCycleEnded {
		a.tr.add("ANN %s %d", noticeNames[n], cycle)
		return
	}
	a.tr.add("ANN %s", noticeNames[n])
}

// traceStore records each write, then makes it against the database.
type traceStore struct {
	*gamesql.OlympiadStore
	tr *trace
}

func (s traceStore) SaveCycle(ctx context.Context, cycle int32) error {
	s.tr.add("DB save_cycle %d", cycle)
	return s.OlympiadStore.SaveCycle(ctx, cycle)
}

func (s traceStore) SaveNobles(ctx context.Context, nobles map[int32]olympiad.Noble) error {
	s.tr.add("DB save_nobles %s", nobleList(nobles))
	return s.OlympiadStore.SaveNobles(ctx, nobles)
}

func (s traceStore) DeleteNobles(ctx context.Context) error {
	s.tr.add("DB delete_nobles")
	return s.OlympiadStore.DeleteNobles(ctx)
}

func (s traceStore) SnapshotMonth(ctx context.Context) error {
	s.tr.add("DB snapshot_month")
	return s.OlympiadStore.SnapshotMonth(ctx)
}

// nobleList lists nobles as id:points by id, or "-" for none.
func nobleList(nobles map[int32]olympiad.Noble) string {
	if len(nobles) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(nobles))
	for _, id := range slices.Sorted(maps.Keys(nobles)) {
		parts = append(parts, fmt.Sprintf("%d:%d", id, nobles[id].Points))
	}
	return strings.Join(parts, ",")
}

// instant formats t as the probe prints it: UTC, milliseconds only when
// there are some.
func instant(t time.Time) string {
	return strings.Replace(t.UTC().Format("2006-01-02T15:04:05.000Z"), ".000Z", "Z", 1)
}

// timelineScenario is one probe run: the clock's start, how many distinct
// instants to step through, and the stored cycle and noble points.
type timelineScenario struct {
	name     string
	start    string
	instants int
	cycle    int
	nobles   map[int32]int
}

// timelineScenarios are the probe's runs, in its order.
var timelineScenarios = []timelineScenario{
	{"validation start, nobles, crosses month end", "2026-10-02T12:00:00Z", 30, 4, map[int32]int{1001: 10, 1002: 0}},
	{"inside competition", "2026-10-31T20:00:00Z", 12, 1, map[int32]int{1001: 5}},
	{"yesterday's window past midnight", "2026-11-30T23:59:59.500Z", 10, 7, nil},
	{"competition hour, minute passed, not grant day", "2026-11-03T18:30:00Z", 10, 2, map[int32]int{1001: 1}},
	{"first of month in competition", "2026-12-01T19:00:00Z", 10, 2, map[int32]int{1001: 1}},
}

// TestOlympiadCalendarTimeline runs the calendar on a virtual clock against
// the database, with the default configuration in UTC, and compares every
// announcement, database write and the state after each instant with
// testdata/timeline.golden.
//
// The golden file is the output of a probe of the reference calendar
// (aCis 409, model/olympiad/Olympiad.java: reschedule, getDelay,
// executeTask, endOlympiad, checkPendingGames, schedulePeriodEnd,
// revalidatePeriod, init, startCompetition, setNextNoblePointsUpdate,
// setNewOlympiadEnd, startNewCycle, deleteNobles, copied with
// System.currentTimeMillis and Calendar.getInstance read from a virtual
// clock, ThreadPool.schedule replaced by stepping that clock to the soonest
// delay, the database, broadcast and game manager calls replaced by trace
// lines, and no match ever running). Its one deviation: init renews the
// Olympiad end once reached rather than once passed, as the Go calendar
// does; on a clock that never moves the reference's comparison repeats the
// steps due at the end forever (#3280). The probe and how to regenerate the
// golden file are in testdata/oracle (OlyProbe.java, README.md).
//
// The runs cover a start in the validation period, inside the window,
// inside the previous day's window just before midnight, during the start
// hour once its minute has passed, and on the first of a month; each
// crosses at least one month end.
func TestOlympiadCalendarTimeline(t *testing.T) {
	want, err := os.ReadFile("testdata/timeline.golden")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, sc := range timelineScenarios {
		got = append(got, "# "+sc.name)
		t.Run(sc.name, func(t *testing.T) { got = append(got, runTimeline(t, sc)...) })
	}
	if g, w := strings.Join(got, "\n")+"\n", string(want); g != w {
		gl, wl := strings.Split(g, "\n"), strings.Split(w, "\n")
		for i := range min(len(gl), len(wl)) {
			if gl[i] != wl[i] {
				t.Fatalf("timeline line %d = %q, want %q", i+1, gl[i], wl[i])
			}
		}
		t.Fatalf("timeline has %d lines, want %d", len(gl), len(wl))
	}
}

// runTimeline runs one scenario on its own database and returns its trace.
func runTimeline(t *testing.T, sc timelineScenario) []string {
	t.Helper()
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	seedNobles(t, db, sc.cycle, sc.nobles)

	start, err := time.Parse(time.RFC3339Nano, sc.start)
	if err != nil {
		t.Fatal(err)
	}
	tr := &trace{}
	loop := sim.NewInline(start)
	o := olympiad.New(olympiad.DefaultConfig(), traceStore{gamesql.NewOlympiadStore(db), tr}, nil, traceAnnouncer{tr}, loop.NewQueue("olympiad"), zerolog.Nop())
	if err := o.Restore(ctx); err != nil {
		t.Fatalf("%s: restore: %v", sc.name, err)
	}
	o.Start()
	loop.Run()
	state := func() {
		period := "COMPETITION"
		if o.Period() == olympiad.Validation {
			period = "VALIDATION"
		}
		held := map[int32]olympiad.Noble{}
		for id := range sc.nobles {
			if n, ok := o.Noble(id); ok {
				held[id] = n
			}
		}
		tr.add("@%s cycle=%d period=%s nobles=%s", instant(loop.Now()), o.Cycle(), period, nobleList(held))
	}
	state()
	for range sc.instants {
		at, ok := loop.NextTimer()
		if !ok {
			break
		}
		loop.Advance(at.Sub(loop.Now()))
		state()
	}
	lines := slices.Clone(tr.lines)
	o.Stop(ctx)
	return lines
}

// seedNobles stores cycle and, for each noble, a character and its record
// with the given points.
func seedNobles(t *testing.T, db *sql.DB, cycle int, nobles map[int32]int) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "INSERT INTO server_memo (var, value) VALUES ('olympiad_cycle', ?)", fmt.Sprint(cycle)); err != nil {
		t.Fatalf("seed cycle: %v", err)
	}
	for id, points := range nobles {
		if _, err := db.ExecContext(ctx, "INSERT INTO characters (account_name, obj_Id, char_name, nobless) VALUES ('olympiad', ?, ?, 1)", id, fmt.Sprintf("Noble%d", id)); err != nil {
			t.Fatalf("seed noble %d: %v", id, err)
		}
		if _, err := db.ExecContext(ctx, "INSERT INTO olympiad_nobles (char_id, class_id, olympiad_points) VALUES (?, 88, ?)", id, points); err != nil {
			t.Fatalf("seed record %d: %v", id, err)
		}
	}
}
