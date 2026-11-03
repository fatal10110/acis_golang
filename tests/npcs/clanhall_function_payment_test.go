package npcs

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// storedAdena is the adena Talker's items rows hold, as a restart would
// read it: no item tick runs here, so only a write the function change
// made itself has landed.
func (w *hallManagerWorld) storedAdena(t *testing.T) int64 {
	t.Helper()
	var n int64
	if err := w.srv.DB.QueryRowContext(context.Background(),
		"SELECT COALESCE(SUM(count), 0) FROM items WHERE owner_id = ? AND item_id = ?", w.player, item.AdenaID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestClanHallFunctionRentLandsTheRentersAdenaFirst: once a rented or
// changed function's row is in the database, so is the renter's adena as
// the fee left it, without waiting for the item tick, whether the fee took
// part of the stack or all of it (#3712).
func TestClanHallFunctionRentLandsTheRentersAdenaFirst(t *testing.T) {
	t.Parallel()
	t.Run("part of the stack, then a change", func(t *testing.T) {
		t.Parallel()
		w := bootHallManager(t, hallManagerSetup{adena: 100_000})
		w.run(t, "manage recovery hp 4")
		if row, ok := w.storedFunctions(t)[1]; !ok || row[0] != 80 {
			t.Fatalf("stored HP function = %v (%v), want level 80", row, ok)
		}
		if got := w.storedAdena(t); got != 99_000 {
			t.Fatalf("stored adena with the rent's row landed = %d, want 99000", got)
		}
		w.run(t, "manage recovery hp 10")
		if row := w.storedFunctions(t)[1]; row[0] != 200 {
			t.Fatalf("stored HP function = %v, want level 200", row)
		}
		if got := w.storedAdena(t); got != 91_500 {
			t.Fatalf("stored adena with the change's row landed = %d, want 91500", got)
		}
	})
	t.Run("the whole stack", func(t *testing.T) {
		t.Parallel()
		w := bootHallManager(t, hallManagerSetup{adena: 1000})
		w.run(t, "manage recovery hp 4")
		if row, ok := w.storedFunctions(t)[1]; !ok || row[0] != 80 {
			t.Fatalf("stored HP function = %v (%v), want level 80", row, ok)
		}
		if got := w.storedAdena(t); got != 0 {
			t.Fatalf("stored adena with the rent's row landed = %d, want the stack deleted", got)
		}
	})
}

// TestClanHallFunctionRentWithTheAdenaWriteHeldStoresNoTerm: while the
// renter's adena row cannot land, the rented function's row does not land
// either, so a crash leaves the database with neither the term nor the
// payment, never the term and the adena (#3712).
func TestClanHallFunctionRentWithTheAdenaWriteHeldStoresNoTerm(t *testing.T) {
	t.Parallel()
	fault := &gameservertest.ItemFlushFault{}
	w := bootHallManager(t, hallManagerSetup{adena: 100_000, extra: []gameservertest.Option{gameservertest.WithItemFlushFault(fault)}})
	fault.Arm()
	t.Cleanup(fault.Disarm)
	frames := w.run(t, "manage recovery hp 4")
	if got := pageIn(t, frames); got != fill(managerPage(t, "functions-apply_confirmed.htm"), "%objectId%", w.oid()) {
		t.Fatalf("rent page =\n%s", got)
	}
	if got := w.adena(t); got != 99_000 {
		t.Fatalf("live adena = %d, want 99000", got)
	}
	if rows := w.storedFunctions(t); len(rows) != 0 {
		t.Fatalf("stored functions = %v with the renter's adena write failing, want none", rows)
	}
	if got := w.storedAdena(t); got != 100_000 {
		t.Fatalf("stored adena = %d, want the 100000 the failed write left", got)
	}
}
