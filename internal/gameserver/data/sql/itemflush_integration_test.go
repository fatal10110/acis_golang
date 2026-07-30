//go:build integration

package sql

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// TestItemFlushStore_Flush_Atomic pins the atomicity acceptance criterion:
// a flush interrupted mid-write leaves the items table with either all of
// that flush's changes or none of them. The augmentation group's write
// deliberately violates the augmentations table's unsigned item_oid column
// after the items group's write has already gone through inside the same
// transaction; Flush must roll the whole thing back rather than leave the
// item row committed.
func TestItemFlushStore_Flush_Atomic(t *testing.T) {
	ctx := context.Background()
	db := sqltest.NewDB(t)
	store := NewItemFlushStore(db)

	batch := item.FlushBatch{
		Saves: []*item.Instance{
			{ObjectID: 0x10000101, TemplateID: 10, OwnerID: 0x10000001, Count: 5, Location: item.LocationInventory, ManaLeft: -1},
			{ObjectID: 0x10000102, TemplateID: 10, OwnerID: 0x10000001, Count: 3, Location: item.LocationInventory, ManaLeft: -1},
		},
		AugmentationSaves: []item.FlushAugmentationSave{
			// item_oid is UNSIGNED NOT NULL; -1 is rejected by the driver,
			// failing this statement after the items group already ran.
			{ObjectID: -1, Augmentation: item.Augmentation{Attributes: 1, SkillID: 1, SkillLevel: 1}},
		},
	}

	if err := store.Flush(ctx, batch); err == nil {
		t.Fatal("Flush() with an invalid augmentation row succeeded, want error")
	}

	var count int
	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM items WHERE object_id IN (?, ?)", 0x10000101, 0x10000102)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count items: %v", err)
	}
	if count != 0 {
		t.Errorf("items rows after failed flush = %d, want 0 (rolled back)", count)
	}
}

// TestItemFlushStore_Flush_MultiRow proves a batch spanning saves, deletes,
// augmentation saves/deletes, and a pet delete lands in one call.
func TestItemFlushStore_Flush_MultiRow(t *testing.T) {
	ctx := context.Background()
	db := sqltest.NewDB(t)
	store := NewItemFlushStore(db)

	// Seed rows the batch's deletes must remove.
	if _, err := db.ExecContext(ctx,
		"INSERT INTO items (owner_id, object_id, item_id, count, enchant_level, loc, loc_data, custom_type1, custom_type2, mana_left, time) VALUES (?,?,?,?,?,?,?,?,?,?,?)",
		0x10000001, 0x10000201, 10, 1, 0, item.LocationInventory.String(), 0, 0, 0, -1, 0); err != nil {
		t.Fatalf("seed item: %v", err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO augmentations (item_oid, attributes, skill_id, skill_level) VALUES (?,?,?,?)",
		0x10000202, 1, 1, 1); err != nil {
		t.Fatalf("seed augmentation: %v", err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO pets (name, level, curHp, curMp, exp, sp, fed, item_obj_id) VALUES (?,?,?,?,?,?,?,?)",
		"pet", 1, 100, 100, 0, 0, 100, 0x10000203); err != nil {
		t.Fatalf("seed pet: %v", err)
	}

	batch := item.FlushBatch{
		Saves: []*item.Instance{
			{ObjectID: 0x10000301, TemplateID: 10, OwnerID: 0x10000001, Count: 7, Location: item.LocationInventory, ManaLeft: -1},
		},
		Deletes: []int32{0x10000201},
		AugmentationSaves: []item.FlushAugmentationSave{
			{ObjectID: 0x10000302, Augmentation: item.Augmentation{Attributes: 42, SkillID: 5, SkillLevel: 2}},
		},
		AugmentationDeletes: []int32{0x10000202},
		PetDeletes:          []int32{0x10000203},
	}

	if err := store.Flush(ctx, batch); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT count FROM items WHERE object_id = ?", 0x10000301).Scan(&count); err != nil {
		t.Fatalf("read saved item: %v", err)
	}
	if count != 7 {
		t.Errorf("saved item count = %d, want 7", count)
	}

	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM items WHERE object_id = ?", 0x10000201).Scan(&n); err != nil {
		t.Fatalf("count deleted item: %v", err)
	}
	if n != 0 {
		t.Errorf("deleted item rows = %d, want 0", n)
	}

	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM augmentations WHERE item_oid = ?", 0x10000302).Scan(&n); err != nil {
		t.Fatalf("count saved augmentation: %v", err)
	}
	if n != 1 {
		t.Errorf("saved augmentation rows = %d, want 1", n)
	}

	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM augmentations WHERE item_oid = ?", 0x10000202).Scan(&n); err != nil {
		t.Fatalf("count deleted augmentation: %v", err)
	}
	if n != 0 {
		t.Errorf("deleted augmentation rows = %d, want 0", n)
	}

	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM pets WHERE item_obj_id = ?", 0x10000203).Scan(&n); err != nil {
		t.Fatalf("count deleted pet: %v", err)
	}
	if n != 0 {
		t.Errorf("deleted pet rows = %d, want 0", n)
	}
}
