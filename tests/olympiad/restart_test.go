package olympiad

import (
	"context"
	"slices"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// discard drops every announcement.
type discard struct{}

func (discard) Announce(olympiad.Notice, int32) {}

// newOlympiad restores an Olympiad on a virtual clock at at, its writes
// queued on a persistence worker of its own.
func newOlympiad(t *testing.T, store olympiad.Store, at time.Time) (*olympiad.Olympiad, *sim.Inline) {
	t.Helper()
	worker := persist.New(zerolog.Nop())
	t.Cleanup(func() { _ = worker.Close(context.Background()) })
	loop := sim.NewInline(at)
	o := olympiad.New(olympiad.DefaultConfig(), store, worker, discard{}, loop.NewQueue("olympiad"), zerolog.Nop())
	if err := o.Restore(context.Background()); err != nil {
		t.Fatalf("restore: %v", err)
	}
	return o, loop
}

// TestOlympiadRestoreWithNothingStored pins a first boot: with no cycle
// stored the Olympiad is in its first cycle and holds no record, and the
// save on stop stores the cycle and writes no record.
func TestOlympiadRestoreWithNothingStored(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	o, _ := newOlympiad(t, gamesql.NewOlympiadStore(db), time.Now())
	if got := o.Cycle(); got != 1 {
		t.Fatalf("cycle = %d, want 1", got)
	}
	o.Stop(ctx)
	var value string
	if err := db.QueryRowContext(ctx, "SELECT value FROM server_memo WHERE var = 'olympiad_cycle'").Scan(&value); err != nil || value != "1" {
		t.Fatalf("stored cycle = %q, %v; want \"1\"", value, err)
	}
	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM olympiad_nobles").Scan(&n); err != nil || n != 0 {
		t.Fatalf("olympiad_nobles rows = %d, %v; want 0", n, err)
	}
}

// TestOlympiadRecordsSurviveRestart pins the records' round trip through a
// restart: the restore loads every record whose character exists, with the
// character's name; the save on stop writes them back, re-creating a row
// removed meanwhile; and the next restore loads the same records and
// cycle.
func TestOlympiadRecordsSurviveRestart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	seedNobles(t, db, 6, map[int32]int{1001: 33})
	for _, stmt := range []string{
		"UPDATE olympiad_nobles SET competitions_done = 4, competitions_won = 2, competitions_lost = 1, competitions_drawn = 1, rewarded = 1 WHERE char_id = 1001",
		// A record whose character was deleted is not loaded.
		"INSERT INTO olympiad_nobles (char_id, class_id, olympiad_points) VALUES (1999, 88, 70)",
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed %q: %v", stmt, err)
		}
	}
	want := olympiad.Noble{ClassID: 88, Name: "Noble1001", Points: 33, CompDone: 4, CompWon: 2, CompLost: 1, CompDrawn: 1, Rewarded: true}

	store := gamesql.NewOlympiadStore(db)
	// Inside the competition window, so starting the calendar wipes
	// nothing before the stop.
	at := time.Date(2026, 10, 7, 20, 0, 0, 0, time.Local)
	o, loop := newOlympiad(t, store, at)
	o.Start()
	loop.Run()
	if got, ok := o.Noble(1001); !ok || got != want {
		t.Fatalf("restored record = %+v, %v; want %+v", got, ok, want)
	}
	if _, ok := o.Noble(1999); ok {
		t.Fatal("restored the record of a deleted character")
	}
	if got := o.Cycle(); got != 6 {
		t.Fatalf("restored cycle = %d, want 6", got)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM olympiad_nobles WHERE char_id = 1001"); err != nil {
		t.Fatal(err)
	}
	o.Stop(ctx)

	again, _ := newOlympiad(t, store, at)
	if got, ok := again.Noble(1001); !ok || got != want {
		t.Fatalf("record after restart = %+v, %v; want %+v", got, ok, want)
	}
	if got := again.Cycle(); got != 6 {
		t.Fatalf("cycle after restart = %d, want 6", got)
	}
}

// TestOlympiadStopCancelsPendingStep pins that once stopped the calendar
// runs no further step: the pending competition end never fires, so after
// the stop's own save nothing is announced, saved or wiped.
func TestOlympiadStopCancelsPendingStep(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	seedNobles(t, db, 2, map[int32]int{1001: 5})
	tr := &trace{}
	at := time.Date(2026, 10, 7, 20, 0, 0, 0, time.UTC)
	loop := sim.NewInline(at)
	o := olympiad.New(olympiad.DefaultConfig(), traceStore{gamesql.NewOlympiadStore(db), tr}, nil, traceAnnouncer{tr}, loop.NewQueue("olympiad"), zerolog.Nop())
	if err := o.Restore(ctx); err != nil {
		t.Fatal(err)
	}
	o.Start()
	loop.Run()
	o.Stop(ctx)
	loop.Advance(48 * time.Hour)
	want := []string{"ANN game_started", "DB save_cycle 2", "DB save_nobles 1001:5"}
	if !slices.Equal(tr.lines, want) {
		t.Fatalf("trace = %q, want %q", tr.lines, want)
	}
}
