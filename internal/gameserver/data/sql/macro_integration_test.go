package sql

import (
	"context"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/macro"
)

// TestMacroStoreRoundTrip covers create, update in place, reload through a
// second store, a missing owner, and delete against the shipped schema.
func TestMacroStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	store := NewMacroStore(db)
	const owner = 0x10000001

	if rows, err := store.ListByOwner(ctx, owner); err != nil || len(rows) != 0 {
		t.Fatalf("ListByOwner for an owner without macros = %+v, %v; want none", rows, err)
	}

	first := macro.Macro{
		ID: 1001, Icon: 2, Name: "Heal", Description: "self", Acronym: "H",
		Commands: []macro.Command{{Type: macro.CommandSkill, D1: 1011}, {Type: macro.CommandAction, D1: 2, Text: "/a"}},
	}
	second := macro.Macro{ID: 1000, Icon: 1, Name: "Buff", Commands: nil}
	for _, m := range []macro.Macro{first, second} {
		if err := store.Save(ctx, owner, m); err != nil {
			t.Fatalf("Save %d: %v", m.ID, err)
		}
	}
	first.Name, first.Icon = "Heal2", 4
	if err := store.Save(ctx, owner, first); err != nil {
		t.Fatalf("Save update: %v", err)
	}

	rows, err := NewMacroStore(db).ListByOwner(ctx, owner)
	if err != nil {
		t.Fatalf("ListByOwner: %v", err)
	}
	want := []macro.Row{
		{ID: 1000, Icon: 1, Name: "Buff", Commands: "", CommandsValid: true},
		{ID: 1001, Icon: 4, Name: "Heal2", Description: "self", Acronym: "H", Commands: "1,1011,0;3,2,0,/a;", CommandsValid: true},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("ListByOwner = %+v, want %+v", rows, want)
	}

	if err := store.Delete(ctx, owner, 1000); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	rows, err = store.ListByOwner(ctx, owner)
	if err != nil || len(rows) != 1 || rows[0].ID != 1001 {
		t.Fatalf("ListByOwner after delete = %+v, %v; want only 1001", rows, err)
	}
}

// TestMacroStoreReadsNullColumns pins the NULL handling: text columns read
// as empty and a NULL command column is reported, so the restore can stop
// there.
func TestMacroStoreReadsNullColumns(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	if _, err := db.ExecContext(ctx, `INSERT INTO character_macroses (char_obj_id, id) VALUES (7, 1000)`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	rows, err := NewMacroStore(db).ListByOwner(ctx, 7)
	if err != nil {
		t.Fatalf("ListByOwner: %v", err)
	}
	if want := []macro.Row{{ID: 1000}}; !reflect.DeepEqual(rows, want) {
		t.Fatalf("ListByOwner = %+v, want %+v", rows, want)
	}
}

// TestCharacterPurgeDeletesMacros pins that deleting a character drops its
// macros with it.
func TestCharacterPurgeDeletesMacros(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	chars := NewCharacterStore(db)
	macros := NewMacroStore(db)
	c := testCharacter(0x10000021, "Purged")
	if err := chars.Create(ctx, c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := macros.Save(ctx, c.ID, macro.Macro{ID: 1000, Name: "M"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := chars.Purge(ctx, c.ID); err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if rows, err := macros.ListByOwner(ctx, c.ID); err != nil || len(rows) != 0 {
		t.Fatalf("macros after purge = %+v, %v; want none", rows, err)
	}
}
