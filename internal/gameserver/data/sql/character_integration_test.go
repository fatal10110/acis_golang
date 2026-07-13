//go:build integration

package sql

import (
	"context"
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

func testCharacter(objectID int32, name string) *player.Character {
	return &player.Character{
		ID:          objectID,
		AccountName: "acct1",
		Name:        name,
		ClassID:     0,
		BaseClassID: 0,
		Race:        player.RaceHuman,
		Sex:         player.SexMale,
		Level:       1,
		MaxHP:       80, CurHP: 80,
		MaxCP: 32, CurCP: 32,
		MaxMP: 30, CurMP: 30,
		Face: 1, HairStyle: 2, HairColor: 3,
		Exp: 0, SP: 0,
		AccessLevel: 0,
	}
}

func TestCharacterStore_Get_NotFound(t *testing.T) {
	ctx := context.Background()
	store := NewCharacterStore(sqltest.NewDB(t))

	_, err := store.Get(ctx, 0x10000001)
	if !errors.Is(err, ErrCharacterNotFound) {
		t.Fatalf("Get() error = %v, want ErrCharacterNotFound", err)
	}
}

func TestCharacterStore_CreateAndReadBack(t *testing.T) {
	ctx := context.Background()
	store := NewCharacterStore(sqltest.NewDB(t))

	c := testCharacter(0x10000001, "Newbie")
	if err := store.Create(ctx, c); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	got, err := store.Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if got.AccountName != c.AccountName || got.Name != c.Name || got.ClassID != c.ClassID ||
		got.Race != c.Race || got.Sex != c.Sex || got.Level != c.Level ||
		got.MaxHP != c.MaxHP || got.CurHP != c.CurHP || got.MaxMP != c.MaxMP || got.MaxCP != c.MaxCP ||
		got.Face != c.Face || got.HairStyle != c.HairStyle || got.HairColor != c.HairColor {
		t.Fatalf("Get() after create = %+v, want match to %+v", got, c)
	}
	// Columns not part of the initial insert keep the schema's own
	// defaults until something else sets them.
	if got.Location.X != 0 || got.Location.Y != 0 || got.Location.Z != 0 || got.Heading != 0 {
		t.Errorf("Get() after create has non-zero position/heading: %+v", got)
	}
	if got.DeleteAt != 0 {
		t.Errorf("Get() after create DeleteAt = %d, want 0", got.DeleteAt)
	}
}

// TestCharacterStore_RestartReload simulates a server restart: a second
// store instance, opened against the same database, must see exactly what
// the first one wrote.
func TestCharacterStore_RestartReload(t *testing.T) {
	ctx := context.Background()
	db := sqltest.NewDB(t)
	first := NewCharacterStore(db)

	c := testCharacter(0x10000001, "Newbie")
	if err := first.Create(ctx, c); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	second := NewCharacterStore(db)
	got, err := second.Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("Get() after reload unexpected error: %v", err)
	}
	if got.Name != c.Name {
		t.Fatalf("Get() after reload Name = %q, want %q", got.Name, c.Name)
	}
}

func TestCharacterStore_ListByAccount(t *testing.T) {
	ctx := context.Background()
	store := NewCharacterStore(sqltest.NewDB(t))

	a1 := testCharacter(0x10000001, "Alpha")
	a2 := testCharacter(0x10000002, "Beta")
	a2.AccountName = "acct1"
	other := testCharacter(0x10000003, "Gamma")
	other.AccountName = "acct2"

	for _, c := range []*player.Character{a1, a2, other} {
		if err := store.Create(ctx, c); err != nil {
			t.Fatalf("Create(%q) unexpected error: %v", c.Name, err)
		}
	}

	got, err := store.ListByAccount(ctx, "acct1")
	if err != nil {
		t.Fatalf("ListByAccount() unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListByAccount() returned %d characters, want 2", len(got))
	}
	if got[0].ID != a1.ID || got[1].ID != a2.ID {
		t.Fatalf("ListByAccount() order = [%d,%d], want [%d,%d]", got[0].ID, got[1].ID, a1.ID, a2.ID)
	}
}

func TestCharacterStore_ListByAccount_Empty(t *testing.T) {
	ctx := context.Background()
	store := NewCharacterStore(sqltest.NewDB(t))

	got, err := store.ListByAccount(ctx, "ghost")
	if err != nil {
		t.Fatalf("ListByAccount() unexpected error: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("ListByAccount() = %v, want empty non-nil slice", got)
	}
}

func TestCharacterStore_CountByAccount_CaseInsensitive(t *testing.T) {
	ctx := context.Background()
	store := NewCharacterStore(sqltest.NewDB(t))

	c := testCharacter(0x10000001, "Newbie")
	c.AccountName = "Player1"
	if err := store.Create(ctx, c); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	n, err := store.CountByAccount(ctx, "player1")
	if err != nil {
		t.Fatalf("CountByAccount() unexpected error: %v", err)
	}
	if n != 1 {
		t.Errorf("CountByAccount() = %d, want 1", n)
	}
}

func TestCharacterStore_NameTaken_CaseInsensitive(t *testing.T) {
	ctx := context.Background()
	store := NewCharacterStore(sqltest.NewDB(t))

	c := testCharacter(0x10000001, "Newbie")
	if err := store.Create(ctx, c); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	taken, err := store.NameTaken(ctx, "newBIE")
	if err != nil {
		t.Fatalf("NameTaken() unexpected error: %v", err)
	}
	if !taken {
		t.Error("NameTaken(\"newBIE\") = false, want true")
	}

	taken, err = store.NameTaken(ctx, "someoneElse")
	if err != nil {
		t.Fatalf("NameTaken() unexpected error: %v", err)
	}
	if taken {
		t.Error("NameTaken(\"someoneElse\") = true, want false")
	}
}

func TestCharacterStore_SetDeleteAt(t *testing.T) {
	ctx := context.Background()
	store := NewCharacterStore(sqltest.NewDB(t))

	c := testCharacter(0x10000001, "Newbie")
	if err := store.Create(ctx, c); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	if err := store.SetDeleteAt(ctx, c.ID, 1_800_000_000_000); err != nil {
		t.Fatalf("SetDeleteAt() unexpected error: %v", err)
	}
	got, err := store.Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if got.DeleteAt != 1_800_000_000_000 {
		t.Errorf("DeleteAt = %d, want 1800000000000", got.DeleteAt)
	}

	if err := store.SetDeleteAt(ctx, c.ID, 0); err != nil {
		t.Fatalf("SetDeleteAt(restore) unexpected error: %v", err)
	}
	got, err = store.Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if got.DeleteAt != 0 {
		t.Errorf("DeleteAt after restore = %d, want 0", got.DeleteAt)
	}
}

func TestCharacterStore_Delete(t *testing.T) {
	ctx := context.Background()
	store := NewCharacterStore(sqltest.NewDB(t))

	c := testCharacter(0x10000001, "Newbie")
	if err := store.Create(ctx, c); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	deleted, err := store.Delete(ctx, c.ID)
	if err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}
	if !deleted {
		t.Error("Delete() on existing character deleted = false, want true")
	}
	if _, err := store.Get(ctx, c.ID); !errors.Is(err, ErrCharacterNotFound) {
		t.Fatalf("Get() after delete: got err %v, want ErrCharacterNotFound", err)
	}

	deleted, err = store.Delete(ctx, c.ID)
	if err != nil {
		t.Fatalf("Delete() second call unexpected error: %v", err)
	}
	if deleted {
		t.Error("Delete() on missing character deleted = true, want false")
	}
}
