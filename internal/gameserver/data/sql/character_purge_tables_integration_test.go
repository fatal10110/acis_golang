package sql

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// TestCharacterStore_PurgeClearsMemoQuestHennaAndSubclassRows seeds a row in
// each of the per-character tables whose delete has no other assertion, next
// to the same rows of a second character, and checks the purge removes only
// the purged character's rows.
func TestCharacterStore_PurgeClearsMemoQuestHennaAndSubclassRows(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	store := NewCharacterStore(db)

	gone := testCharacter(0x10000001, "Newbie")
	kept := testCharacter(0x10000002, "Bystander")
	for _, c := range []*player.Character{gone, kept} {
		if err := store.Create(ctx, c); err != nil {
			t.Fatalf("Create(%d) unexpected error: %v", c.ID, err)
		}
	}
	seeds := []struct {
		table, column, insert string
	}{
		{"character_memo", "charId", "INSERT INTO character_memo (charId, var, val) VALUES (?, 'k', 'v')"},
		{"character_quests", "charId", "INSERT INTO character_quests (charId, name, var, value) VALUES (?, 'Q001', 'cond', '1')"},
		{"character_hennas", "char_obj_id", "INSERT INTO character_hennas (char_obj_id, symbol_id, slot, class_index) VALUES (?, 1, 1, 0)"},
		{"character_subclasses", "char_obj_id", "INSERT INTO character_subclasses (char_obj_id, class_id, exp, sp, level, class_index) VALUES (?, 10, 0, 0, 40, 1)"},
	}
	for _, s := range seeds {
		for _, id := range []int32{gone.ID, kept.ID} {
			if _, err := db.ExecContext(ctx, s.insert, id); err != nil {
				t.Fatalf("seed %s for %d: %v", s.table, id, err)
			}
		}
	}

	if deleted, err := store.Purge(ctx, gone.ID); err != nil || !deleted {
		t.Fatalf("Purge() = %v, %v; want true, nil", deleted, err)
	}
	for _, s := range seeds {
		query := "SELECT COUNT(*) FROM " + s.table + " WHERE " + s.column + " = ?"
		if n := countRows(t, db, query, gone.ID); n != 0 {
			t.Errorf("%s rows after purge = %d, want 0", s.table, n)
		}
		if n := countRows(t, db, query, kept.ID); n != 1 {
			t.Errorf("%s rows of the other character after purge = %d, want 1", s.table, n)
		}
	}
}
