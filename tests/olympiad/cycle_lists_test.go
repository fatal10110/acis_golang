package olympiad

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
)

// TestOlympiadCycleWipeClearsWaitingLists pins the waiting lists across a
// cycle (#5117): they outlive the window's close and the Olympiad's end,
// and go at the cycle wipe with the records, so in the next cycle's window
// a noble is no longer told it is registered, and registering again gives
// it a new record with the starting points.
func TestOlympiadCycleWipeClearsWaitingLists(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	seedNobles(t, db, 4, map[int32]int{1001: 30, 1002: 30})
	o, loop, worker := calendarOlympiad(t, db, time.Date(2026, 10, 31, 20, 0, 0, 0, time.UTC), &trace{})
	o.Start()
	loop.Run()
	applicant := func(id int32) olympiad.Applicant {
		return olympiad.Applicant{ObjectID: id, Name: "Noble", BaseClass: 88, Noble: true}
	}
	if r := o.Register(applicant(1001), olympiad.Classed); r != olympiad.Registered {
		t.Fatalf("register 1001 classed = %v", r)
	}
	if r := o.Register(applicant(1002), olympiad.NonClassed); r != olympiad.Registered {
		t.Fatalf("register 1002 non-classed = %v", r)
	}

	// The window closes at midnight and the Olympiad ends at noon: the
	// lists stay.
	loop.Advance(16 * time.Hour)
	if got := o.Period(); got != olympiad.Validation {
		t.Fatalf("period = %v, want Validation", got)
	}
	if c, n := o.WaitingList(); c != 1 || n != 1 {
		t.Fatalf("waiting lists in the validation period = %d classed, %d non-classed; want 1 and 1", c, n)
	}

	// The validation period closes at noon the next day: cycle 5 opens
	// with neither records nor lists.
	loop.Advance(24 * time.Hour)
	if got := o.Cycle(); got != 5 {
		t.Fatalf("cycle = %d, want 5", got)
	}
	if c, n := o.WaitingList(); c != 0 || n != 0 {
		t.Fatalf("waiting lists after the wipe = %d classed, %d non-classed; want none", c, n)
	}
	for _, id := range []int32{1001, 1002} {
		if o.IsRegistered(id) {
			t.Fatalf("%d still registered after the wipe", id)
		}
		if _, ok := o.Noble(id); ok {
			t.Fatalf("%d kept its record after the wipe", id)
		}
	}
	flush(t, worker)
	if got := countRows(t, db, "olympiad_nobles"); got != 0 {
		t.Fatalf("olympiad_nobles rows after the wipe = %d, want 0", got)
	}

	// Cycle 5's first window opens at 18:00.
	loop.Advance(7 * time.Hour)
	if got := o.Period(); got != olympiad.Competition {
		t.Fatalf("period at 19:00 = %v, want Competition", got)
	}
	if r := o.Register(applicant(1001), olympiad.Classed); r != olympiad.Registered {
		t.Fatalf("register 1001 in cycle 5 = %v, want Registered", r)
	}
	if n, ok := o.Noble(1001); !ok || n.Points != olympiad.DefaultConfig().StartPoints {
		t.Fatalf("record of 1001 in cycle 5 = %+v, %v; want the starting points", n, ok)
	}
	o.Stop(ctx)
	if got := countRows(t, db, "olympiad_nobles"); got != 1 {
		t.Fatalf("olympiad_nobles rows after the stop save = %d, want 1", got)
	}
}
