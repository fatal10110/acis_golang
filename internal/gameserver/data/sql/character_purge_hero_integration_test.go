package sql

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// TestCharacterStore_PurgeClearsHeroRow seeds a heroes row for the purged
// character and for a second one, and checks the purge removes only the purged
// character's row.
func TestCharacterStore_PurgeClearsHeroRow(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	store := NewCharacterStore(db)

	gone := testCharacter(0x10000011, "FormerHero")
	kept := testCharacter(0x10000012, "StillHero")
	for _, c := range []*player.Character{gone, kept} {
		if err := store.Create(ctx, c); err != nil {
			t.Fatalf("Create(%d) unexpected error: %v", c.ID, err)
		}
		if _, err := db.ExecContext(ctx, "INSERT INTO heroes (char_id, class_id, count, played, active) VALUES (?, 88, 1, 1, 1)", c.ID); err != nil {
			t.Fatalf("seed heroes for %d: %v", c.ID, err)
		}
	}

	if deleted, err := store.Purge(ctx, gone.ID); err != nil || !deleted {
		t.Fatalf("Purge() = %v, %v; want true, nil", deleted, err)
	}
	const query = "SELECT COUNT(*) FROM heroes WHERE char_id = ?"
	if n := countRows(t, db, query, gone.ID); n != 0 {
		t.Errorf("heroes rows after purge = %d, want 0", n)
	}
	if n := countRows(t, db, query, kept.ID); n != 1 {
		t.Errorf("heroes rows of the other character after purge = %d, want 1", n)
	}
}
