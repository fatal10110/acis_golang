package clanhall

import (
	"testing"
	"time"

	hallmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// TestRemoveAllStopsFees: a hall changing hands drops its functions, and
// their fees stop with them (#5018): however long the clock runs, nobody is
// charged for a function the hall no longer rents.
func TestRemoveAllStopsFees(t *testing.T) {
	t.Parallel()
	start := time.Unix(1_700_000_000, 0)
	day := (24 * time.Hour).Milliseconds()
	store := &feeStore{rows: []StoredFunction{{HallID: feeHall, Function: Function{
		Type: hallmodel.FuncRestoreHP, Level: 80, Lease: 1000, Rate: day, EndTime: start.UnixMilli() + day,
	}}}}
	f := restoredFunctions(t, store)
	clock := sim.NewInline(start)
	treasury := &feeTreasury{}
	f.Start(clock.NewQueue("clanhall-functions"), treasury)
	if _, armed := clock.NextTimer(); !armed {
		t.Fatal("no fee timer armed before the removal")
	}

	f.RemoveAll(feeHall)
	for range 5 {
		clock.Advance(24 * time.Hour)
	}
	if got := treasury.charges(); got != 0 {
		t.Fatalf("fees charged after the removal = %d, want 0", got)
	}
	if at, armed := clock.NextTimer(); armed {
		t.Fatalf("fee timer still armed for %v, want none", at)
	}
	if got := f.Level(feeHall, hallmodel.FuncRestoreHP); got != 0 {
		t.Fatalf("level after the removal = %d, want 0", got)
	}
}
