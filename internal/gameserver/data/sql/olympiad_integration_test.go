package sql

import (
	"context"
	"maps"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
)

// TestOlympiadStoreStatus pins the status's server_memo rows: none stored
// reads as not found, a save inserts both, a later save replaces them, and
// a cycle stored alone reads with no validation period running.
func TestOlympiadStoreStatus(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	store := NewOlympiadStore(db)

	if _, found, err := store.LoadStatus(ctx); err != nil || found {
		t.Fatalf("LoadStatus() on an empty table = found %v, err %v; want not found", found, err)
	}
	for _, st := range []olympiad.Status{{Cycle: 3, ValidationEnd: 1793534400000}, {Cycle: 4}} {
		if err := store.SaveStatus(ctx, st); err != nil {
			t.Fatalf("SaveStatus(%+v): %v", st, err)
		}
		got, found, err := store.LoadStatus(ctx)
		if err != nil || !found || got != st {
			t.Fatalf("LoadStatus() after SaveStatus(%+v) = %+v, %v, %v", st, got, found, err)
		}
	}
	for name, want := range map[string]string{"olympiad_cycle": "4", "olympiad_valid_end": "0"} {
		var value string
		if err := db.QueryRowContext(ctx, "SELECT value FROM server_memo WHERE var = ?", name).Scan(&value); err != nil || value != want {
			t.Fatalf("server_memo %s = %q, %v; want %q", name, value, err, want)
		}
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM server_memo WHERE var = 'olympiad_valid_end'"); err != nil {
		t.Fatal(err)
	}
	if got, found, err := store.LoadStatus(ctx); err != nil || !found || got != (olympiad.Status{Cycle: 4}) {
		t.Fatalf("LoadStatus() with the cycle alone = %+v, %v, %v", got, found, err)
	}
}

// TestOlympiadStoreStatusOutOfRange pins that a stored cycle beyond 32
// bits, or a validation end beyond 64, fails the load rather than
// wrapping, as the reference's integer parse rejects it.
func TestOlympiadStoreStatusOutOfRange(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	store := NewOlympiadStore(db)
	for _, tc := range []struct{ name, value string }{
		{"olympiad_cycle", "4294967297"},
		{"olympiad_cycle", "2147483648"},
		{"olympiad_cycle", "-2147483649"},
		{"olympiad_cycle", "abc"},
		{"olympiad_valid_end", "9223372036854775808"},
		{"olympiad_valid_end", "abc"},
	} {
		if _, err := db.ExecContext(ctx, "DELETE FROM server_memo WHERE var LIKE 'olympiad_%'"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, "INSERT INTO server_memo (var, value) VALUES (?, ?)", tc.name, tc.value); err != nil {
			t.Fatal(err)
		}
		if got, found, err := store.LoadStatus(ctx); err == nil {
			t.Fatalf("LoadStatus() with %s %q stored = %+v, %v, nil; want an error", tc.name, tc.value, got, found)
		}
	}
}

// TestOlympiadStoreNobles pins the olympiad_nobles round trip: a save
// inserts new records and updates stored ones, keeping their stored class;
// a load joins the character's current name and skips records whose
// character no longer exists; the month's copy takes every stored record
// but its rewarded flag; a delete removes them all.
func TestOlympiadStoreNobles(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	store := NewOlympiadStore(db)
	chars := NewCharacterStore(db)
	for _, c := range []struct {
		id   int32
		name string
	}{{0x10000001, "Alpha"}, {0x10000002, "Beta"}} {
		if err := chars.Create(ctx, testCharacter(c.id, c.name)); err != nil {
			t.Fatalf("create %s: %v", c.name, err)
		}
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO olympiad_nobles VALUES (0x10000002, 90, 40, 9, 5, 4, 0, 0), (0x10000099, 88, 70, 1, 1, 0, 0, 0)"); err != nil {
		t.Fatalf("seed olympiad_nobles: %v", err)
	}

	got, err := store.LoadNobles(ctx)
	if err != nil {
		t.Fatalf("LoadNobles(): %v", err)
	}
	want := map[int32]olympiad.Noble{0x10000002: {ClassID: 90, Name: "Beta", Points: 40, CompDone: 9, CompWon: 5, CompLost: 4}}
	if !maps.Equal(got, want) {
		t.Fatalf("LoadNobles() = %+v, want %+v (the record without a character is skipped)", got, want)
	}

	if err := store.SaveNobles(ctx, map[int32]olympiad.Noble{
		0x10000001: {ClassID: 88, Name: "Alpha", Points: 18},
		0x10000002: {ClassID: 12, Name: "Beta", Points: 41, CompDone: 10, CompWon: 6, CompLost: 4, CompDrawn: 1, Rewarded: true},
	}); err != nil {
		t.Fatalf("SaveNobles(): %v", err)
	}
	got, err = store.LoadNobles(ctx)
	if err != nil {
		t.Fatalf("LoadNobles() after save: %v", err)
	}
	want = map[int32]olympiad.Noble{
		0x10000001: {ClassID: 88, Name: "Alpha", Points: 18},
		0x10000002: {ClassID: 90, Name: "Beta", Points: 41, CompDone: 10, CompWon: 6, CompLost: 4, CompDrawn: 1, Rewarded: true},
	}
	if !maps.Equal(got, want) {
		t.Fatalf("LoadNobles() after save = %+v, want %+v (an update keeps the stored class)", got, want)
	}

	if err := store.SnapshotMonth(ctx); err != nil {
		t.Fatalf("SnapshotMonth(): %v", err)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM olympiad_nobles_eom"); n != 3 {
		t.Fatalf("olympiad_nobles_eom rows = %d, want 3", n)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM olympiad_nobles_eom WHERE char_id = ? AND class_id = 90 AND olympiad_points = 41 AND competitions_done = 10 AND competitions_won = 6 AND competitions_lost = 4 AND competitions_drawn = 1", 0x10000002); n != 1 {
		t.Fatalf("olympiad_nobles_eom row for Beta not copied")
	}
	// A second copy replaces the first.
	if err := store.SnapshotMonth(ctx); err != nil {
		t.Fatalf("SnapshotMonth() again: %v", err)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM olympiad_nobles_eom"); n != 3 {
		t.Fatalf("olympiad_nobles_eom rows after a second copy = %d, want 3", n)
	}

	if err := store.DeleteNobles(ctx); err != nil {
		t.Fatalf("DeleteNobles(): %v", err)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM olympiad_nobles"); n != 0 {
		t.Fatalf("olympiad_nobles rows after DeleteNobles = %d, want 0", n)
	}
}

// TestCharacterStoreNoble pins characters.nobless: a stored 1 loads as a
// noble, and a save writes the flag back.
func TestCharacterStoreNoble(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	store := NewCharacterStore(db)
	c := testCharacter(0x10000001, "Noble")
	if err := store.Create(ctx, c); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	if got, err := store.Get(ctx, c.ID); err != nil || got.IsNoble() {
		t.Fatalf("Get() of a new character: noble %v, err %v; want not noble", got != nil && got.IsNoble(), err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET nobless = 1 WHERE obj_Id = ?", c.ID); err != nil {
		t.Fatalf("set nobless: %v", err)
	}
	got, err := store.Get(ctx, c.ID)
	if err != nil || !got.IsNoble() {
		t.Fatalf("Get() with nobless = 1: noble %v, err %v; want noble", err == nil && got.IsNoble(), err)
	}
	got.SetNoble(false)
	if err := store.Save(ctx, got.SaveState()); err != nil {
		t.Fatalf("Save(): %v", err)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM characters WHERE obj_Id = ? AND nobless = 0", c.ID); n != 1 {
		t.Fatal("Save() of a character no longer noble left nobless set")
	}
}

// TestCharacterStorePurgeOlympiadRecord pins that deleting a character
// deletes its olympiad_nobles row and leaves the others.
func TestCharacterStorePurgeOlympiadRecord(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	store := NewCharacterStore(db)
	c := testCharacter(0x10000001, "Gone")
	if err := store.Create(ctx, c); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO olympiad_nobles (char_id, class_id, olympiad_points) VALUES (?, 88, 18), (?, 88, 18)", c.ID, c.ID+1); err != nil {
		t.Fatalf("seed olympiad_nobles: %v", err)
	}
	if _, err := store.Purge(ctx, c.ID); err != nil {
		t.Fatalf("Purge(): %v", err)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM olympiad_nobles WHERE char_id = ?", c.ID); n != 0 {
		t.Fatalf("olympiad_nobles rows of the purged character = %d, want 0", n)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM olympiad_nobles"); n != 1 {
		t.Fatalf("olympiad_nobles rows left = %d, want the other character's 1", n)
	}
}
