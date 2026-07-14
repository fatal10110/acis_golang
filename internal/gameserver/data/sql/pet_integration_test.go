//go:build integration

package sql

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
)

func TestPetStore_DeleteByItemObjectID(t *testing.T) {
	ctx := context.Background()
	db := sqltest.NewDB(t)
	store := NewPetStore(db)

	if _, err := db.ExecContext(ctx, `INSERT INTO pets (item_obj_id, name, level) VALUES (?,?,?)`, 0x10000101, "Wolf", 1); err != nil {
		t.Fatalf("insert pet row: %v", err)
	}
	if err := store.DeleteByItemObjectID(ctx, 0x10000101); err != nil {
		t.Fatalf("DeleteByItemObjectID() unexpected error: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pets WHERE item_obj_id = ?`, 0x10000101).Scan(&count); err != nil {
		t.Fatalf("count pet rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("pet rows after delete = %d, want 0", count)
	}
}

func TestPetStore_Get_NotFound(t *testing.T) {
	ctx := context.Background()
	store := NewPetStore(sqltest.NewDB(t))

	_, ok, err := store.Get(ctx, 0x10000999)
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if ok {
		t.Errorf("Get() on a collar with no saved pet should report not found")
	}
}

func TestPetStore_SaveAndGet(t *testing.T) {
	ctx := context.Background()
	store := NewPetStore(sqltest.NewDB(t))

	st := pet.State{Name: "Wolf", Level: 15, CurHP: 250, CurMP: 40, Exp: 123456, SP: 7, Fed: 88}
	if err := store.Save(ctx, 0x10000101, st); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}

	got, ok, err := store.Get(ctx, 0x10000101)
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("Get() reported not found, want found")
	}
	if got != st {
		t.Errorf("Get() = %+v, want %+v", got, st)
	}
}

func TestPetStore_SaveUpserts(t *testing.T) {
	ctx := context.Background()
	conn := sqltest.NewDB(t)
	store := NewPetStore(conn)

	if err := store.Save(ctx, 0x10000101, pet.State{Name: "Wolf", Level: 1, Fed: 100}); err != nil {
		t.Fatalf("Save(insert) unexpected error: %v", err)
	}
	updated := pet.State{Name: "Wolf", Level: 20, CurHP: 500, CurMP: 60, Exp: 999, SP: 3, Fed: 12}
	if err := store.Save(ctx, 0x10000101, updated); err != nil {
		t.Fatalf("Save(update) unexpected error: %v", err)
	}

	got, ok, err := store.Get(ctx, 0x10000101)
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("Get() reported not found, want found")
	}
	if got != updated {
		t.Errorf("Get() = %+v, want %+v", got, updated)
	}

	var count int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM pets WHERE item_obj_id = ?`, 0x10000101).Scan(&count); err != nil {
		t.Fatalf("count pet rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("pet rows for collar after upsert = %d, want 1 (update, not a duplicate insert)", count)
	}
}
