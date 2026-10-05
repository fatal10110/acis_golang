package clanhall

import (
	"testing"
	"time"

	hallmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// Reference: ClanHall.updateFunction creates a missing function with
// endTime = now + rate and saves it, removes one whose new level and lease
// are both 0, and otherwise calls ClanHallFunction.refreshFunction: the new
// fee and level, endTime = now + the function's own (final) rate, saved,
// and the fee task rescheduled after that rate.

// startedFunctions is feeHall's functions over store, running on an inline
// clock from start.
func startedFunctions(t *testing.T, store *feeStore, start time.Time) (*Functions, *sim.Inline, *feeTreasury) {
	t.Helper()
	f := restoredFunctions(t, store)
	clock := sim.NewInline(start)
	treasury := &feeTreasury{}
	f.Start(clock.NewQueue("clanhall-functions"), treasury)
	return f, clock, treasury
}

// TestUpdateRentsChangesAndCancels: a new function's first term runs from
// now for its rate and is charged when it ends; a change keeps the first
// term's length, restarts the term now and charges the new lease when it
// ends; level and lease 0 remove it, stop its fee and delete its row.
func TestUpdateRentsChangesAndCancels(t *testing.T) {
	t.Parallel()
	start := time.Unix(1_700_000_000, 0)
	day := 24 * time.Hour
	store := &feeStore{}
	f, clock, treasury := startedFunctions(t, store, start)

	f.Update(feeHall, hallmodel.FuncRestoreHP, 80, 1000, 3*day.Milliseconds())
	got, ok := f.Get(feeHall, hallmodel.FuncRestoreHP)
	want := Function{Type: hallmodel.FuncRestoreHP, Level: 80, Lease: 1000, Rate: 3 * day.Milliseconds(), EndTime: start.Add(3 * day).UnixMilli()}
	if !ok || got != want {
		t.Fatalf("rented = %+v, %v; want %+v", got, ok, want)
	}
	if store.saved() != 1 {
		t.Fatalf("rows saved = %d, want 1", store.saved())
	}

	clock.Advance(day)
	f.Update(feeHall, hallmodel.FuncRestoreHP, 140, 1750, day.Milliseconds())
	got, _ = f.Get(feeHall, hallmodel.FuncRestoreHP)
	want = Function{Type: hallmodel.FuncRestoreHP, Level: 140, Lease: 1750, Rate: 3 * day.Milliseconds(), EndTime: start.Add(4 * day).UnixMilli()}
	if got != want {
		t.Fatalf("changed = %+v, want %+v (the first term's length kept)", got, want)
	}
	// The first term's end passes: its fee timer was replaced.
	clock.Advance(2 * day)
	if n := treasury.charges(); n != 0 {
		t.Fatalf("fees at the first term's end = %d, want 0", n)
	}
	clock.Advance(day)
	if n := treasury.charges(); n != 1 || treasury.paid[0] != 1750 {
		t.Fatalf("fees at the new term's end = %v, want [1750]", treasury.paid)
	}

	f.Update(feeHall, hallmodel.FuncRestoreHP, 0, 0, 0)
	if _, ok := f.Get(feeHall, hallmodel.FuncRestoreHP); ok {
		t.Fatal("cancelled function still rented")
	}
	if store.deletes != 1 {
		t.Fatalf("rows deleted = %d, want 1", store.deletes)
	}
	clock.Advance(10 * day)
	if n := treasury.charges(); n != 1 {
		t.Fatalf("fees after the cancel = %d, want 1", n)
	}
	if at, armed := clock.NextTimer(); armed {
		t.Fatalf("fee timer still armed for %v", at)
	}
}

// TestUpdateLevelZeroRentsNothing: cancelling a function the hall does not
// rent rents and stores nothing.
func TestUpdateLevelZeroRentsNothing(t *testing.T) {
	t.Parallel()
	store := &feeStore{}
	f, clock, _ := startedFunctions(t, store, time.Unix(1_700_000_000, 0))
	f.Update(feeHall, hallmodel.FuncTeleport, 0, 0, 0)
	if _, ok := f.Get(feeHall, hallmodel.FuncTeleport); ok {
		t.Fatal("a level 0 function was rented")
	}
	if store.saved() != 0 || store.deletes != 0 {
		t.Fatalf("writes = %d saves, %d deletes; want none", store.saved(), store.deletes)
	}
	if at, armed := clock.NextTimer(); armed {
		t.Fatalf("fee timer armed for %v", at)
	}
}

// TestUpdateOnFreeHallChargesNothing: a function rented on a hall no clan
// owns is kept but never charged.
func TestUpdateOnFreeHallChargesNothing(t *testing.T) {
	t.Parallel()
	start := time.Unix(1_700_000_000, 0)
	halls, err := hallmodel.NewTable([]*hallmodel.Hall{{ID: int(feeHall), Alias: "moonstone"}, {ID: 99, Alias: "free"}})
	if err != nil {
		t.Fatal(err)
	}
	f := New(halls, nil, feeOwners{}, &feeStore{}, nil, zerolog.Nop())
	clock := sim.NewInline(start)
	treasury := &feeTreasury{}
	f.Start(clock.NewQueue("clanhall-functions"), treasury)
	f.Update(99, hallmodel.FuncDecoCurtains, 1, 2000, time.Hour.Milliseconds())
	if _, ok := f.Get(99, hallmodel.FuncDecoCurtains); !ok {
		t.Fatal("function on a free hall not kept")
	}
	if at, armed := clock.NextTimer(); armed {
		t.Fatalf("fee timer armed for %v on a free hall", at)
	}
}
