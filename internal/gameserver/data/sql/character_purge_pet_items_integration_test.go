package sql

import (
	"context"
	"database/sql"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// TestCharacterStorePurgeRemovesPetItems deletes a character whose pet
// carries items. Those rows are saved under the pet's collar, not under the
// character (see itemcontainer.NewPetInventory), yet the reference's delete
// removes them with the character's own items, since it keys them on the
// player: the purge must remove them, and their augmentation, too. Another
// character's pet items stay.
func TestCharacterStorePurgeRemovesPetItems(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	store := NewCharacterStore(db)

	const (
		keeperID       int32 = 0x10000701
		keeperCollar   int32 = 0x10000711
		keeperPetItem  int32 = 0x10000712
		keeperPetArmor int32 = 0x10000713
		otherID        int32 = 0x10000702
		otherCollar    int32 = 0x10000721
		otherPetItem   int32 = 0x10000722
	)
	for _, c := range []struct {
		id   int32
		name string
	}{{keeperID, "PetKeeper"}, {otherID, "PetBystander"}} {
		if err := store.Create(ctx, testCharacter(c.id, c.name)); err != nil {
			t.Fatalf("Create(%s): %v", c.name, err)
		}
	}
	seedItemRow(t, db, keeperID, keeperCollar, item.LocationInventory)
	seedItemRow(t, db, keeperCollar, keeperPetItem, item.LocationPet)
	seedItemRow(t, db, keeperCollar, keeperPetArmor, item.LocationPetEquip)
	seedItemRow(t, db, otherID, otherCollar, item.LocationInventory)
	seedItemRow(t, db, otherCollar, otherPetItem, item.LocationPet)
	for _, collar := range []int32{keeperCollar, otherCollar} {
		if _, err := db.ExecContext(ctx, "INSERT INTO pets (item_obj_id, name) VALUES (?, NULL)", collar); err != nil {
			t.Fatalf("seed pets row %d: %v", collar, err)
		}
	}
	for _, oid := range []int32{keeperPetItem, otherPetItem} {
		if _, err := db.ExecContext(ctx, "INSERT INTO augmentations (item_oid, skill_id, skill_level) VALUES (?,?,?)", oid, 1, 1); err != nil {
			t.Fatalf("seed augmentation %d: %v", oid, err)
		}
	}

	if deleted, err := store.Purge(ctx, keeperID); err != nil || !deleted {
		t.Fatalf("Purge() = %v, %v, want true, nil", deleted, err)
	}

	if n := countRows(t, db, "SELECT COUNT(*) FROM items WHERE owner_id IN (?, ?)", keeperID, keeperCollar); n != 0 {
		t.Errorf("purged character's item and pet item rows = %d, want 0", n)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM pets WHERE item_obj_id = ?", keeperCollar); n != 0 {
		t.Errorf("purged character's pets rows = %d, want 0", n)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM augmentations WHERE item_oid = ?", keeperPetItem); n != 0 {
		t.Errorf("purged character's pet item augmentations = %d, want 0", n)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM items WHERE object_id IN (?, ?)", otherCollar, otherPetItem); n != 2 {
		t.Errorf("other character's collar and pet item rows = %d, want 2", n)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM pets WHERE item_obj_id = ?", otherCollar); n != 1 {
		t.Errorf("other character's pets rows = %d, want 1", n)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM augmentations WHERE item_oid = ?", otherPetItem); n != 1 {
		t.Errorf("other character's pet item augmentations = %d, want 1", n)
	}
}

func seedItemRow(t *testing.T, db *sql.DB, ownerID, objectID int32, loc item.Location) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(),
		"INSERT INTO items (owner_id, object_id, item_id, count, enchant_level, loc, loc_data, custom_type1, custom_type2, mana_left, time) VALUES (?,?,?,?,?,?,?,?,?,?,?)",
		ownerID, objectID, 57, 1, 0, loc.String(), 0, 0, 0, -1, 0); err != nil {
		t.Fatalf("seed item %d: %v", objectID, err)
	}
}
