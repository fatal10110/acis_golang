package sql

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func testCharacter(objectID int32, name string) *player.Character {
	c := &player.Character{
		ID:          objectID,
		AccountName: "acct1",
		Name:        name,
		ClassID:     0,
		BaseClassID: 0,
		Race:        player.RaceHuman,
		Sex:         player.SexMale,
		CharLevel:   1,
		Face:        1, HairStyle: 2, HairColor: 3,
		Exp: 0, SP: 0,
		AccessLevel: 0,
		Location:    location.Location{X: -56733, Y: -113459, Z: -690},
		LastHeading: 32768,
	}
	c.SetResourceValues(player.Resources{
		MaxHP: 80, CurrentHP: 80,
		MaxCP: 32, CurrentCP: 32,
		MaxMP: 30, CurrentMP: 30,
	})
	return c
}

func TestCharacterStore_Get_NotFound(t *testing.T) {
	ctx := context.Background()
	store := NewCharacterStore(sqltest.SharedDB(t))

	_, err := store.Get(ctx, 0x10000001)
	if !errors.Is(err, ErrCharacterNotFound) {
		t.Fatalf("Get() error = %v, want ErrCharacterNotFound", err)
	}
}

func TestCharacterStore_CreateAndReadBack(t *testing.T) {
	ctx := context.Background()
	store := NewCharacterStore(sqltest.SharedDB(t))

	c := testCharacter(0x10000001, "Newbie")
	if err := store.Create(ctx, c); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	got, err := store.Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	gotRes := got.ResourceValues()
	wantRes := c.ResourceValues()
	if got.AccountName != c.AccountName || got.Name != c.Name || got.ClassID != c.ClassID ||
		got.Race != c.Race || got.Sex != c.Sex || got.CharLevel != c.CharLevel ||
		gotRes != wantRes ||
		got.Face != c.Face || got.HairStyle != c.HairStyle || got.HairColor != c.HairColor {
		t.Fatalf("Get() after create = %+v, want match to %+v", got, c)
	}
	if got.Location != c.Location || got.LastHeading != c.LastHeading {
		t.Errorf("Get() after create position/heading = %v/%d, want %v/%d", got.Location, got.LastHeading, c.Location, c.LastHeading)
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
	db := sqltest.SharedDB(t)
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
	store := NewCharacterStore(sqltest.SharedDB(t))

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
	store := NewCharacterStore(sqltest.SharedDB(t))

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
	store := NewCharacterStore(sqltest.SharedDB(t))

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
	store := NewCharacterStore(sqltest.SharedDB(t))

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
	store := NewCharacterStore(sqltest.SharedDB(t))

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

func TestCharacterStore_Purge(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	store := NewCharacterStore(db)

	c := testCharacter(0x10000001, "Newbie")
	if err := store.Create(ctx, c); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	seedOwnedRows(t, db, c.ID)

	deleted, err := store.Purge(ctx, c.ID)
	if err != nil {
		t.Fatalf("Purge() unexpected error: %v", err)
	}
	if !deleted {
		t.Error("Purge() on existing character deleted = false, want true")
	}
	if _, err := store.Get(ctx, c.ID); !errors.Is(err, ErrCharacterNotFound) {
		t.Fatalf("Get() after purge: got err %v, want ErrCharacterNotFound", err)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM items WHERE owner_id = ?", c.ID); n != 0 {
		t.Errorf("item rows after purge = %d, want 0", n)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM character_shortcuts WHERE char_obj_id = ?", c.ID); n != 0 {
		t.Errorf("shortcut rows after purge = %d, want 0", n)
	}

	deleted, err = store.Purge(ctx, c.ID)
	if err != nil {
		t.Fatalf("Purge() second call unexpected error: %v", err)
	}
	if deleted {
		t.Error("Purge() on missing character deleted = true, want false")
	}
}

// TestCharacterStore_Purge_Atomic pins the atomicity contract: a purge that
// fails after the character and item deletes have already run inside the
// transaction leaves every row in place. Dropping character_shortcuts
// guarantees that last statement fails regardless of the server's sql_mode.
func TestCharacterStore_Purge_Atomic(t *testing.T) {
	ctx := context.Background()
	// Uses its own container, not SharedDB: this test drops a table out
	// from under the schema, which would corrupt every other test sharing
	// the package's container.
	db := sqltest.NewDB(t)
	store := NewCharacterStore(db)

	c := testCharacter(0x10000001, "Newbie")
	if err := store.Create(ctx, c); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	seedOwnedRows(t, db, c.ID)

	if _, err := db.ExecContext(ctx, "DROP TABLE character_shortcuts"); err != nil {
		t.Fatalf("drop character_shortcuts table: %v", err)
	}

	if _, err := store.Purge(ctx, c.ID); err == nil {
		t.Fatal("Purge() against a missing character_shortcuts table succeeded, want error")
	}

	if _, err := store.Get(ctx, c.ID); err != nil {
		t.Errorf("Get() after failed purge: got err %v, want the character row retained", err)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM items WHERE owner_id = ?", c.ID); n != 1 {
		t.Errorf("item rows after failed purge = %d, want 1 (rolled back)", n)
	}
}

// seedOwnedRows writes one item and one shortcut owned by ownerID.
func seedOwnedRows(t *testing.T, db *sql.DB, ownerID int32) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(),
		"INSERT INTO items (owner_id, object_id, item_id, count, enchant_level, loc, loc_data, custom_type1, custom_type2, mana_left, time) VALUES (?,?,?,?,?,?,?,?,?,?,?)",
		ownerID, 0x10000101, 10, 1, 0, item.LocationInventory.String(), 0, 0, 0, -1, 0); err != nil {
		t.Fatalf("seed item: %v", err)
	}
	if _, err := db.ExecContext(context.Background(),
		"INSERT INTO character_shortcuts (char_obj_id, slot, page, type, id, level, class_index) VALUES (?,?,?,?,?,?,?)",
		ownerID, 0, 0, "ITEM", 0x10000101, -1, 0); err != nil {
		t.Fatalf("seed shortcut: %v", err)
	}
}

func countRows(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}
