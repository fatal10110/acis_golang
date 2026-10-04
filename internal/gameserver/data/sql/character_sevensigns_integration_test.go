package sql

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
)

// TestCharacterStoreSevenSignsDungeon pins characters.isin7sdungeon
// (Player.java:4119, :4332): a new character is in no dungeon, a stored 1
// loads in one, and every save writes the membership back so the next Get
// reads it.
func TestCharacterStoreSevenSignsDungeon(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	store := NewCharacterStore(db)
	c := testCharacter(0x10000001, "Delver")
	if err := store.Create(ctx, c); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	if got, err := store.Get(ctx, c.ID); err != nil || got.In7sDungeon() {
		t.Fatalf("Get() of a new character: in dungeon %v, err %v; want out", got != nil && got.In7sDungeon(), err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET isin7sdungeon = 1 WHERE obj_Id = ?", c.ID); err != nil {
		t.Fatalf("set isin7sdungeon: %v", err)
	}
	got, err := store.Get(ctx, c.ID)
	if err != nil || !got.In7sDungeon() {
		t.Fatalf("Get() with isin7sdungeon = 1: in dungeon %v, err %v; want in", err == nil && got.In7sDungeon(), err)
	}
	for _, in := range []bool{false, true} {
		got.SetIn7sDungeon(in)
		if err := store.Save(ctx, got.SaveState()); err != nil {
			t.Fatalf("Save(): %v", err)
		}
		want := 0
		if in {
			want = 1
		}
		if n := countRows(t, db, "SELECT COUNT(*) FROM characters WHERE obj_Id = ? AND isin7sdungeon = ?", c.ID, want); n != 1 {
			t.Fatalf("Save() in dungeon %v did not store isin7sdungeon = %d", in, want)
		}
		reloaded, err := store.Get(ctx, c.ID)
		if err != nil || reloaded.In7sDungeon() != in {
			t.Fatalf("Get() after Save() in dungeon %v: in dungeon %v, err %v", in, err == nil && reloaded.In7sDungeon(), err)
		}
	}
}
