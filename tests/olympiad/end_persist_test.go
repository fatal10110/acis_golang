package olympiad

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/hero"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// heroOlympiad restores, on a virtual clock at at, an Olympiad against db
// electing its heroes through a hero manager restored from db, both
// writing through one persistence worker of their own.
func heroOlympiad(t *testing.T, db *sql.DB, at time.Time) (*olympiad.Olympiad, *sim.Inline, *persist.Worker) {
	t.Helper()
	ctx := context.Background()
	worker := persist.New(zerolog.Nop())
	t.Cleanup(func() { _ = worker.Close(context.Background()) })
	loop := sim.NewInline(at)
	heroes := hero.New(gamesql.NewHeroStore(db), nil, nil, worker, olympiad.DefaultConfig().MinMatches, loop.Now, zerolog.Nop())
	if err := heroes.Restore(ctx); err != nil {
		t.Fatalf("restore heroes: %v", err)
	}
	o := olympiad.New(olympiad.DefaultConfig(), gamesql.NewOlympiadStore(db), worker, discard{}, heroes, loop.NewQueue("olympiad"), zerolog.Nop())
	if err := o.Restore(ctx); err != nil {
		t.Fatalf("restore: %v", err)
	}
	return o, loop, worker
}

// seedHeroToBe stores cycle 4 and noble 1001, a Duelist with 60 points
// and enough matches and wins to be elected.
func seedHeroToBe(t *testing.T, db *sql.DB) {
	t.Helper()
	seedNobles(t, db, 4, map[int32]int{1001: 60})
	if _, err := db.ExecContext(context.Background(), "UPDATE olympiad_nobles SET competitions_done = 6, competitions_won = 4 WHERE char_id = 1001"); err != nil {
		t.Fatal(err)
	}
}

// heroCount returns the stored hero count of objectID, 0 when it is no
// hero.
func heroCount(t *testing.T, db *sql.DB, objectID int32) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(), "SELECT count FROM heroes WHERE char_id = ?", objectID).Scan(&n); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	return n
}

func flush(t *testing.T, worker *persist.Worker) {
	t.Helper()
	if err := worker.Flush(context.Background(), persist.ServiceOwner); err != nil {
		t.Fatal(err)
	}
}

func millis(at time.Time) string { return strconv.FormatInt(at.UnixMilli(), 10) }

// TestOlympiadEndCaughtUpAtBoot pins an Olympiad whose stored end passed
// while the server was down (#4695): the start ends it at once, the
// validation period running a day from then, the hero elected and the
// month's standings kept; a restart inside that validation period elects
// no one again.
func TestOlympiadEndCaughtUpAtBoot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	seedHeroToBe(t, db)
	start := time.Date(2026, 10, 31, 20, 0, 0, 0, time.UTC)
	o, loop, _ := heroOlympiad(t, db, start)
	o.Start()
	// Steps the clock passed fire at once.
	loop.Advance(0)
	o.Stop(ctx)
	end := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	if got := memo(t, db, "olympiad_end"); got != millis(end) {
		t.Fatalf("stored Olympiad end = %q, want %q", got, millis(end))
	}

	boot := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	o, loop, worker := heroOlympiad(t, db, boot)
	o.Start()
	// Steps the clock passed fire at once.
	loop.Advance(0)
	flush(t, worker)
	if got := o.Period(); got != olympiad.Validation || o.Cycle() != 4 {
		t.Fatalf("after a boot past the end: period %v, cycle %d; want Validation, 4", got, o.Cycle())
	}
	if got := memo(t, db, "olympiad_valid_end"); got != millis(boot.Add(24*time.Hour)) {
		t.Fatalf("stored validation end = %q, want a day after the boot", got)
	}
	if got := heroCount(t, db, 1001); got != 1 {
		t.Fatalf("hero count of 1001 = %d, want 1", got)
	}
	if got := countRows(t, db, "olympiad_nobles_eom"); got != 1 {
		t.Fatalf("month's standings = %d rows, want 1", got)
	}
	if got, e := memo(t, db, "olympiad_election"), memo(t, db, "olympiad_electing"); got != "1" || e != "0" {
		t.Fatalf("stored election = %q, electing %q; want 1 finished", got, e)
	}
	if n, ok := o.Noble(1001); !ok || n.Points != 60 {
		t.Fatalf("record of 1001 in the validation period = %+v, %v; want 60 points", n, ok)
	}

	o, loop, worker = heroOlympiad(t, db, boot.Add(time.Hour))
	o.Start()
	// Steps the clock passed fire at once.
	loop.Advance(0)
	flush(t, worker)
	if got := o.Period(); got != olympiad.Validation {
		t.Fatalf("after a restart in the validation period: period %v, want Validation", got)
	}
	if got := heroCount(t, db, 1001); got != 1 {
		t.Fatalf("hero count of 1001 after a restart = %d, want 1", got)
	}
	o.Stop(ctx)
}

// TestOlympiadEndElectsOnceWhenTheFinalSaveFails pins an end whose status
// save after the election never lands, as when the server dies right
// after the election (#4695): the next boot finishes the end without
// electing the heroes again.
func TestOlympiadEndElectsOnceWhenTheFinalSaveFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	seedHeroToBe(t, db)
	start := time.Date(2026, 10, 31, 20, 0, 0, 0, time.UTC)
	o, loop, worker := heroOlympiad(t, db, start)
	o.Start()
	// Steps the clock passed fire at once.
	loop.Advance(0)
	// The window closes at midnight, storing the status unmarked.
	loop.Advance(5 * time.Hour)
	flush(t, worker)

	drop := func() {
		if _, err := db.ExecContext(ctx, "DROP TRIGGER IF EXISTS olympiad_end_fault"); err != nil {
			t.Errorf("drop fault trigger: %v", err)
		}
	}
	t.Cleanup(drop)
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER olympiad_end_fault BEFORE UPDATE ON server_memo FOR EACH ROW
		IF NEW.var = 'olympiad_electing' AND NEW.value = '0' AND OLD.value = '1' THEN
			SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'injected status save fault';
		END IF`); err != nil {
		t.Fatalf("create fault trigger: %v", err)
	}
	loop.Advance(11 * time.Hour)
	flush(t, worker)
	if got := o.Period(); got != olympiad.Validation {
		t.Fatalf("period after the end = %v, want Validation", got)
	}
	if got := heroCount(t, db, 1001); got != 1 {
		t.Fatalf("hero count of 1001 after the end = %d, want 1", got)
	}
	if got := memo(t, db, "olympiad_electing"); got != "1" {
		t.Fatalf("stored electing = %q, want the end left unfinished", got)
	}
	drop()

	// The server died there: a fresh boot finishes the end.
	o, loop, worker = heroOlympiad(t, db, start.Add(17*time.Hour))
	o.Start()
	// Steps the clock passed fire at once.
	loop.Advance(0)
	flush(t, worker)
	if got := o.Period(); got != olympiad.Validation || o.Cycle() != 4 {
		t.Fatalf("after the boot: period %v, cycle %d; want Validation, 4", got, o.Cycle())
	}
	if got := heroCount(t, db, 1001); got != 1 {
		t.Fatalf("hero count of 1001 after the boot = %d, want 1 (elected once)", got)
	}
	if got, e := memo(t, db, "olympiad_election"), memo(t, db, "olympiad_electing"); got != "1" || e != "0" {
		t.Fatalf("stored election = %q, electing %q; want 1 finished", got, e)
	}
	if got := countRows(t, db, "olympiad_nobles_eom"); got != 1 {
		t.Fatalf("month's standings = %d rows, want 1", got)
	}
	o.Stop(ctx)
}

// TestOlympiadWeeklyGrantCaughtUpAtBoot pins a weekly grant that fell due
// while the server was down (#4695): the start makes it once, stores it
// with the next grant's time, and a boot after a crash makes it no second
// time.
func TestOlympiadWeeklyGrantCaughtUpAtBoot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	seedNobles(t, db, 2, map[int32]int{1001: 10})
	// 2026-10-01 is a Thursday, so the grants fall on Thursdays at 18:00.
	start := time.Date(2026, 10, 8, 17, 0, 0, 0, time.UTC)
	o, loop, _ := calendarOlympiad(t, db, start, &trace{})
	o.Start()
	// Steps the clock passed fire at once.
	loop.Advance(0)
	o.Stop(ctx)
	if got := memo(t, db, "olympiad_next_pts"); got != millis(start.Add(time.Hour)) {
		t.Fatalf("stored next grant = %q, want 18:00", got)
	}

	o, loop, worker := calendarOlympiad(t, db, start.Add(3*time.Hour), &trace{})
	o.Start()
	// Steps the clock passed fire at once.
	loop.Advance(0)
	if n, _ := o.Noble(1001); n.Points != 13 {
		t.Fatalf("points after a boot past the grant = %d, want 13", n.Points)
	}
	flush(t, worker)
	var points int
	if err := db.QueryRowContext(ctx, "SELECT olympiad_points FROM olympiad_nobles WHERE char_id = 1001").Scan(&points); err != nil || points != 13 {
		t.Fatalf("stored points = %d, %v; want 13", points, err)
	}
	if got := memo(t, db, "olympiad_next_pts"); got != millis(start.Add(7*24*time.Hour+time.Hour)) {
		t.Fatalf("stored next grant = %q, want a week later", got)
	}

	// The server died without its stop save.
	o, loop, _ = calendarOlympiad(t, db, start.Add(4*time.Hour), &trace{})
	o.Start()
	// Steps the clock passed fire at once.
	loop.Advance(0)
	if n, _ := o.Noble(1001); n.Points != 13 {
		t.Fatalf("points after a second boot = %d, want 13", n.Points)
	}
	o.Stop(ctx)
}

// TestOlympiadWeeklyGrantToNoRecordSaved pins a weekly grant that falls due
// with no record held (#4695): its next time is stored at once, so a record
// a match saves afterwards is not granted the missed week again by a boot
// after a crash.
func TestOlympiadWeeklyGrantToNoRecordSaved(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	seedNobles(t, db, 2, nil)
	// 2026-10-01 is a Thursday, so the grants fall on Thursdays at 18:00.
	start := time.Date(2026, 10, 8, 17, 0, 0, 0, time.UTC)
	o, loop, _ := calendarOlympiad(t, db, start, &trace{})
	o.Start()
	loop.Advance(0)
	o.Stop(ctx)

	o, loop, worker := calendarOlympiad(t, db, start.Add(2*time.Hour), &trace{})
	o.Start()
	// The grant the clock passed fires at once, to no record.
	loop.Advance(0)
	flush(t, worker)
	if got := memo(t, db, "olympiad_next_pts"); got != millis(start.Add(7*24*time.Hour+time.Hour)) {
		t.Fatalf("stored next grant = %q, want a week later", got)
	}

	// A match stores the first record, then the server dies without its
	// stop save.
	if _, err := db.ExecContext(ctx, "INSERT INTO characters (account_name, obj_Id, char_name, nobless) VALUES ('olympiad', 1001, 'Noble1001', 1)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO olympiad_nobles (char_id, class_id, olympiad_points, competitions_done) VALUES (1001, 88, 15, 1)"); err != nil {
		t.Fatal(err)
	}
	o, loop, _ = calendarOlympiad(t, db, start.Add(3*time.Hour), &trace{})
	o.Start()
	loop.Advance(0)
	if n, ok := o.Noble(1001); !ok || n.Points != 15 {
		t.Fatalf("points after a boot = %d, %v; want 15", n.Points, ok)
	}
	o.Stop(ctx)
}
