package sql

import (
	"context"
	"slices"
	"sort"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/derby"
)

// TestDerbyStoreRoundTrip pins mdt_bets and mdt_history: the shipped seed
// loads one empty stake per lane and no record, a saved stake replaces the
// lane's row, clearing zeroes every row, a lane with no row gains one, and
// a record reloads with its odds rounded to the column's two decimals.
func TestDerbyStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	store := NewDerbyStore(db)

	bets, err := store.LoadBets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := make([]derby.Bet, 0, 8)
	for lane := 1; lane <= 8; lane++ {
		want = append(want, derby.Bet{Lane: lane})
	}
	if !slices.Equal(sortBets(bets), want) {
		t.Fatalf("seeded bets = %+v, want %+v", bets, want)
	}
	if history, err := store.LoadHistory(ctx); err != nil || len(history) != 0 {
		t.Fatalf("seeded history = %+v, %v; want none", history, err)
	}

	for _, b := range []derby.Bet{{Lane: 3, Amount: 500}, {Lane: 3, Amount: 1500}, {Lane: 9, Amount: 100}} {
		if err := store.SaveBet(ctx, b); err != nil {
			t.Fatal(err)
		}
	}
	reloaded := NewDerbyStore(db)
	bets, err = reloaded.LoadBets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want[2].Amount = 1500
	if got := sortBets(bets); !slices.Equal(got, append(want[:8:8], derby.Bet{Lane: 9, Amount: 100})) {
		t.Fatalf("bets after save = %+v", got)
	}

	if err := reloaded.ClearBets(ctx); err != nil {
		t.Fatal(err)
	}
	bets, err = reloaded.LoadBets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bets {
		if b.Amount != 0 {
			t.Fatalf("bets after clear = %+v, want every stake 0", bets)
		}
	}

	saved := []derby.History{{RaceID: 7, First: 2, Second: 5, OddRate: 3.456}, {RaceID: 12, First: 0, Second: 7, OddRate: 1.25}}
	for _, h := range saved {
		if err := reloaded.SaveHistory(ctx, h); err != nil {
			t.Fatal(err)
		}
	}
	history, err := NewDerbyStore(db).LoadHistory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(history, func(i, j int) bool { return history[i].RaceID < history[j].RaceID })
	wantHistory := []derby.History{{RaceID: 7, First: 2, Second: 5, OddRate: 3.46}, {RaceID: 12, First: 0, Second: 7, OddRate: 1.25}}
	if !slices.Equal(history, wantHistory) {
		t.Fatalf("history = %+v, want %+v", history, wantHistory)
	}
	if err := reloaded.SaveHistory(ctx, saved[0]); err == nil {
		t.Fatal("SaveHistory of a stored race succeeded, want the duplicate key refused")
	}
}

func sortBets(bets []derby.Bet) []derby.Bet {
	out := append([]derby.Bet(nil), bets...)
	sort.Slice(out, func(i, j int) bool { return out[i].Lane < out[j].Lane })
	return out
}
