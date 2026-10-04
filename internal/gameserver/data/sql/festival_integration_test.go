package sql

import (
	"context"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/festival"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// The store reads the shipped seed (cycle 1's blank scores for both
// cabals) and the festival columns of the status row; a save inserts new
// scores and updates the date, score and members of stored ones; the
// status save writes only the festival columns.
func TestFestivalStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := sqltest.NewDB(t)
	store := NewFestivalStore(db)

	scores, err := store.LoadScores(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(scores) != 10 {
		t.Fatalf("seeded scores = %d, want 10", len(scores))
	}
	for _, s := range scores {
		if s.Cycle != 1 || s.Score != 0 || s.Members != "" || s.Date != 0 || (s.Cabal != sevensigns.Dawn && s.Cabal != sevensigns.Dusk) {
			t.Fatalf("seeded score = %+v", s)
		}
	}

	if err := store.SaveScores(ctx, []festival.Score{
		{FestivalID: 2, Cabal: sevensigns.Dawn, Cycle: 1, Date: 1767225600000, Score: 42, Members: "Ann,Bob"},
		{FestivalID: 0, Cabal: sevensigns.Dusk, Cycle: 2, Date: 0, Score: 0, Members: ""},
	}); err != nil {
		t.Fatal(err)
	}
	scores, err = store.LoadScores(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(scores) != 11 {
		t.Fatalf("scores after save = %d, want 11", len(scores))
	}
	want := festival.Score{FestivalID: 2, Cabal: sevensigns.Dawn, Cycle: 1, Date: 1767225600000, Score: 42, Members: "Ann,Bob"}
	if !slices.Contains(scores, want) {
		t.Fatalf("updated score missing from %+v", scores)
	}

	if _, err := db.Exec(`UPDATE seven_signs_status SET current_cycle = 7, festival_cycle = 3, accumulated_bonus4 = 9 WHERE id = 0`); err != nil {
		t.Fatal(err)
	}
	st, found, err := store.LoadStatus(ctx)
	if err != nil || !found || st != (festival.Status{FestivalCycle: 3, Bonuses: [5]int{0, 0, 0, 0, 9}}) {
		t.Fatalf("LoadStatus = (%+v, %v, %v)", st, found, err)
	}
	if err := store.SaveStatus(ctx, festival.Status{FestivalCycle: 5, Bonuses: [5]int{1, 2, 3, 4, 5}}); err != nil {
		t.Fatal(err)
	}
	var cycle, festivalCycle, b0, b4 int
	if err := db.QueryRow(`SELECT current_cycle, festival_cycle, accumulated_bonus0, accumulated_bonus4 FROM seven_signs_status WHERE id = 0`).
		Scan(&cycle, &festivalCycle, &b0, &b4); err != nil {
		t.Fatal(err)
	}
	if cycle != 7 || festivalCycle != 5 || b0 != 1 || b4 != 5 {
		t.Fatalf("status row = cycle %d, festival cycle %d, bonuses %d..%d", cycle, festivalCycle, b0, b4)
	}

	if _, err := db.Exec(`DELETE FROM seven_signs_status`); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.LoadStatus(ctx); err != nil || found {
		t.Fatalf("LoadStatus without a row = (found %v, %v)", found, err)
	}
}
