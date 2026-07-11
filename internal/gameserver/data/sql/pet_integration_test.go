//go:build integration

package sql

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
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
