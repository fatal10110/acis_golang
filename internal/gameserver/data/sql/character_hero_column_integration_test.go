package sql

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
)

// TestCharacterStoreIgnoresHeroColumn pins that a stored characters.hero flag
// makes no hero: hero status comes from the heroes table alone, which the
// selection consults (Player.restore takes it from the hero registry).
func TestCharacterStoreIgnoresHeroColumn(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	store := NewCharacterStore(db)

	c := testCharacter(0x10000001, "Pretender")
	if err := store.Create(ctx, c); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET hero = 1 WHERE obj_Id = ?", c.ID); err != nil {
		t.Fatalf("set hero column: %v", err)
	}

	got, err := store.Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if got.IsHero() {
		t.Error("Get() restored hero from the characters column")
	}
	listed, err := store.ListByAccount(ctx, c.AccountName)
	if err != nil {
		t.Fatalf("ListByAccount() unexpected error: %v", err)
	}
	if len(listed) != 1 || listed[0].IsHero() {
		t.Errorf("ListByAccount() = %d characters, hero %v; want one non-hero", len(listed), len(listed) == 1 && listed[0].IsHero())
	}
}
