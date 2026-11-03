package olympiad

import (
	"context"
	"database/sql"
	"slices"
	"strconv"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// calendarOlympiad restores an Olympiad on a virtual clock at at, against
// db through a persistence worker of its own, recording its announcements
// in tr.
func calendarOlympiad(t *testing.T, db *sql.DB, at time.Time, tr *trace) (*olympiad.Olympiad, *sim.Inline, *persist.Worker) {
	t.Helper()
	worker := persist.New(zerolog.Nop())
	t.Cleanup(func() { _ = worker.Close(context.Background()) })
	loop := sim.NewInline(at)
	o := olympiad.New(olympiad.DefaultConfig(), gamesql.NewOlympiadStore(db), worker, traceAnnouncer{tr}, nil, loop.NewQueue("olympiad"), zerolog.Nop())
	if err := o.Restore(context.Background()); err != nil {
		t.Fatalf("restore: %v", err)
	}
	return o, loop, worker
}

// memo returns the server_memo value of name, "" when none is stored.
func memo(t *testing.T, db *sql.DB, name string) string {
	t.Helper()
	var value string
	if err := db.QueryRowContext(context.Background(), "SELECT value FROM server_memo WHERE var = ?", name).Scan(&value); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	return value
}

// countRows returns how many rows table holds.
func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestOlympiadEndRunsAtTheMonthEnd drives the calendar through an
// Olympiad's scheduled end at noon on the first: the end is announced, the
// records are kept as the month's standings, the validation period opens no
// window and lets a noble trade its points, and is stored, so a restart
// inside it resumes it. Only once the day of validation is over does the
// next cycle open, with the records cleared.
func TestOlympiadEndRunsAtTheMonthEnd(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	seedNobles(t, db, 4, map[int32]int{1001: 60, 1002: 30})
	tr := &trace{}
	start := time.Date(2026, 10, 31, 20, 0, 0, 0, time.UTC)
	o, loop, worker := calendarOlympiad(t, db, start, tr)
	o.Start()
	loop.Run()

	loop.Advance(16*time.Hour - time.Millisecond)
	if got := o.Period(); got != olympiad.Interval || slices.Contains(tr.lines, "ANN cycle_ended 4") {
		t.Fatalf("before noon on the first: period %v, trace %q; want the Olympiad running", got, tr.lines)
	}
	loop.Advance(time.Millisecond)
	if !slices.Contains(tr.lines, "ANN cycle_ended 4") {
		t.Fatalf("trace at the end = %q, want cycle_ended 4", tr.lines)
	}
	if got := o.Period(); got != olympiad.Validation {
		t.Fatalf("period after the end = %v, want Validation", got)
	}
	if err := worker.Flush(ctx, 0); err != nil {
		t.Fatal(err)
	}
	var points int
	if err := db.QueryRowContext(ctx, "SELECT olympiad_points FROM olympiad_nobles_eom WHERE char_id = 1001").Scan(&points); err != nil || points != 60 {
		t.Fatalf("month's standing of 1001 = %d, %v; want 60", points, err)
	}
	if got := countRows(t, db, "olympiad_nobles_eom"); got != 2 {
		t.Fatalf("month's standings = %d rows, want 2", got)
	}
	wantEnd := strconv.FormatInt(start.Add(40*time.Hour).UnixMilli(), 10)
	if got := memo(t, db, "olympiad_valid_end"); got != wantEnd {
		t.Fatalf("stored validation end = %q, want %q", got, wantEnd)
	}
	if got, err := o.Passes(ctx, 1001, false); err != nil || got != 60000 {
		t.Fatalf("passes in the validation period = %d, %v; want 60000", got, err)
	}

	// The window's hour passes in the validation period: no window opens.
	tr.lines = nil
	loop.Advance(7 * time.Hour)
	if got := o.Period(); got != olympiad.Validation || len(tr.lines) != 0 {
		t.Fatalf("at 19:00 on the first: period %v, trace %q; want Validation and nothing", got, tr.lines)
	}

	// A restart in the validation period resumes it.
	o.Stop(ctx)
	o, loop, worker = calendarOlympiad(t, db, start.Add(23*time.Hour), tr)
	o.Start()
	loop.Run()
	if got := o.Period(); got != olympiad.Validation || o.Cycle() != 4 {
		t.Fatalf("after a restart: period %v, cycle %d; want Validation, 4", got, o.Cycle())
	}
	if n, ok := o.Noble(1002); !ok || n.Points != 30 {
		t.Fatalf("record of 1002 after a restart = %+v, %v; want 30 points", n, ok)
	}

	loop.Advance(17*time.Hour - time.Millisecond)
	if got := o.Cycle(); got != 4 {
		t.Fatalf("cycle before the validation end = %d, want 4", got)
	}
	tr.lines = nil
	loop.Advance(time.Millisecond)
	if got := o.Cycle(); got != 5 || !slices.Contains(tr.lines, "ANN cycle_started 5") {
		t.Fatalf("at the validation end: cycle %d, trace %q; want 5 announced", got, tr.lines)
	}
	if _, ok := o.Noble(1002); ok {
		t.Fatal("the new cycle kept a held record")
	}
	if err := worker.Flush(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, db, "olympiad_nobles"); got != 0 {
		t.Fatalf("olympiad_nobles rows in the new cycle = %d, want 0", got)
	}
	if got, ve := memo(t, db, "olympiad_cycle"), memo(t, db, "olympiad_valid_end"); got != "5" || ve != "0" {
		t.Fatalf("stored status = cycle %q, validation end %q; want 5 and 0", got, ve)
	}
	if got := countRows(t, db, "olympiad_nobles_eom"); got != 2 {
		t.Fatalf("month's standings after the new cycle = %d rows, want the 2 kept", got)
	}
	o.Stop(ctx)
}

// TestOlympiadWeeklyPointsSurvive pins that the weekly grant, at the
// window's opening on the grant day, is kept: the next window's opening
// clears nothing, and the points read back after a restart.
func TestOlympiadWeeklyPointsSurvive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	seedNobles(t, db, 2, map[int32]int{1001: 10})
	// 2026-10-01 is a Thursday, so the grants fall on Thursdays at 18:00.
	start := time.Date(2026, 10, 8, 17, 0, 0, 0, time.UTC)
	o, loop, _ := calendarOlympiad(t, db, start, &trace{})
	o.Start()
	loop.Run()

	loop.Advance(time.Hour)
	if n, _ := o.Noble(1001); n.Points != 13 {
		t.Fatalf("points after the grant = %d, want 13", n.Points)
	}
	loop.Advance(25 * time.Hour)
	if got := o.Period(); got != olympiad.Competition {
		t.Fatalf("period at 19:00 the next day = %v, want Competition", got)
	}
	if n, ok := o.Noble(1001); !ok || n.Points != 13 || o.Cycle() != 2 {
		t.Fatalf("next day: record %+v, %v, cycle %d; want 13 points in cycle 2", n, ok, o.Cycle())
	}
	o.Stop(ctx)

	again, _, _ := calendarOlympiad(t, db, start.Add(26*time.Hour), &trace{})
	if n, ok := again.Noble(1001); !ok || n.Points != 13 {
		t.Fatalf("record after a restart = %+v, %v; want 13 points", n, ok)
	}
}

// TestOlympiadSelectHeroesSchedulesTheNextCycle pins //endoly's end of an
// Olympiad during a competition window: the online player is told the
// cycle ended, the month's standings are kept, the window closes and the
// validation period runs; a day later the player is told the next cycle
// started, and the records are cleared.
func TestOlympiadSelectHeroesSchedulesTheNextCycle(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Login", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithOlympiadCompetition(time.Hour),
		gameservertest.WithOlympiadSeed(func(db *sql.DB) {
			for _, stmt := range []string{
				`UPDATE characters SET nobless = 1 WHERE char_name = 'Login'`,
				`INSERT INTO olympiad_nobles SELECT obj_Id, 88, 70, 6, 4, 2, 0, 0 FROM characters WHERE char_name = 'Login'`,
				`INSERT INTO server_memo (var, value) VALUES ('olympiad_cycle', '8')`,
			} {
				if _, err := db.Exec(stmt); err != nil {
					t.Fatalf("seed %q: %v", stmt, err)
				}
			}
		}))
	login := srv.SoleObjectID(t)
	c := srv.Client
	startInWorld(t, c)

	srv.Olympiad.SelectHeroes()
	srv.Settle(t)
	srv.FlushPersistence(t)
	msgs := systemMessages(t, drainFrames(t, c))
	if !slices.ContainsFunc(msgs, func(m []string) bool {
		return slices.Equal(m, []string{strconv.Itoa(serverpackets.SystemMessageOlympiadPeriodS1HasEnded), "8"})
	}) {
		t.Fatalf("messages at the end = %q, want period 8 ended", msgs)
	}
	if got := srv.Olympiad.Period(); got != olympiad.Validation {
		t.Fatalf("period after //endoly = %v, want Validation", got)
	}
	var points int
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT olympiad_points FROM olympiad_nobles_eom WHERE char_id = ?", login).Scan(&points); err != nil || points != 70 {
		t.Fatalf("month's standing = %d, %v; want 70", points, err)
	}

	srv.Advance(t, 24*time.Hour-time.Second)
	if got := srv.Olympiad.Cycle(); got != 8 {
		t.Fatalf("cycle before the validation end = %d, want 8", got)
	}
	drainFrames(t, c)
	srv.Advance(t, time.Second)
	srv.FlushPersistence(t)
	msgs = systemMessages(t, drainFrames(t, c))
	if !slices.ContainsFunc(msgs, func(m []string) bool {
		return slices.Equal(m, []string{strconv.Itoa(serverpackets.SystemMessageOlympiadPeriodS1HasStarted), "9"})
	}) {
		t.Fatalf("messages at the validation end = %q, want period 9 started", msgs)
	}
	if got := countRows(t, srv.DB, "olympiad_nobles"); got != 0 {
		t.Fatalf("olympiad_nobles rows in cycle 9 = %d, want 0", got)
	}
	if got := memo(t, srv.DB, "olympiad_cycle"); got != "9" {
		t.Fatalf("stored cycle = %q, want 9", got)
	}
}

// systemMessages returns every SystemMessage among frames as its id then
// its parameters.
func systemMessages(t *testing.T, frames [][]byte) [][]string {
	t.Helper()
	var sms [][]byte
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage {
			sms = append(sms, f)
		}
	}
	return sysMsgs(t, sms)
}
