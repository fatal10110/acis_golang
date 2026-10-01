package sql

import (
	"context"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/social/relation"
)

// TestRelationStoreRoundTrip saves relations the way the shutdown save
// does: flagged rows are inserted or have their flags replaced, rows with
// no flags left are deleted, and Load reads them back in key order.
func TestRelationStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := NewRelationStore(sqltest.SharedDB(t))

	if err := store.Save(ctx, []relation.Row{{CharID: 2, FriendID: 9, Relation: 1}, {CharID: 1, FriendID: 5, Relation: 3}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Save(ctx, []relation.Row{{CharID: 1, FriendID: 5, Relation: 0}, {CharID: 2, FriendID: 9, Relation: 5}}); err != nil {
		t.Fatalf("Save again: %v", err)
	}
	got, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []relation.Row{{CharID: 2, FriendID: 9, Relation: 5}}
	if !slices.Equal(got, want) {
		t.Fatalf("Load = %+v, want %+v", got, want)
	}
}

// TestCharacterStoreFindByNameAndNames finds a character by name whatever
// its case, with its stored name and access level, and names characters by
// id, leaving out an id no character has.
func TestCharacterStoreFindByNameAndNames(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	store := NewCharacterStore(db)
	for _, c := range []struct {
		id   int32
		name string
	}{{0x10000001, "Alpha"}, {0x10000002, "Beta"}} {
		if err := store.Create(ctx, testCharacter(c.id, c.name)); err != nil {
			t.Fatalf("Create %s: %v", c.name, err)
		}
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET accesslevel = 1 WHERE obj_Id = ?", 0x10000002); err != nil {
		t.Fatalf("set access level: %v", err)
	}

	ref, ok, err := store.FindByName(ctx, "bETA")
	if err != nil || !ok || ref != (relation.Named{ID: 0x10000002, Name: "Beta", AccessLevel: 1}) {
		t.Fatalf("FindByName(bETA) = %+v, %v, %v; want Beta with access level 1", ref, ok, err)
	}
	if _, ok, err := store.FindByName(ctx, "Gamma"); err != nil || ok {
		t.Fatalf("FindByName(Gamma) found = %v, err = %v; want not found", ok, err)
	}

	names, err := store.Names(ctx, []int32{0x10000001, 0x10000002, 0x10000003})
	if err != nil {
		t.Fatalf("Names: %v", err)
	}
	if len(names) != 2 || names[0x10000001] != "Alpha" || names[0x10000002] != "Beta" {
		t.Fatalf("Names = %v, want Alpha and Beta only", names)
	}
}

// TestCharacterStorePurgeDropsRelations deletes the relation rows naming
// the purged character on either side, and no others.
func TestCharacterStorePurgeDropsRelations(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	store := NewCharacterStore(db)
	c := testCharacter(0x10000005, "Leaver")
	if err := store.Create(ctx, c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := NewRelationStore(db).Save(ctx, []relation.Row{
		{CharID: 0x10000001, FriendID: c.ID, Relation: 1},
		{CharID: c.ID, FriendID: 0x10000009, Relation: 2},
		{CharID: 0x10000001, FriendID: 0x10000009, Relation: 1},
	}); err != nil {
		t.Fatalf("seed relations: %v", err)
	}
	if _, err := store.Purge(ctx, c.ID); err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM character_relations WHERE char_id = ? OR friend_id = ?", c.ID, c.ID); n != 0 {
		t.Errorf("relation rows naming the purged character = %d, want 0", n)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM character_relations"); n != 1 {
		t.Errorf("relation rows left = %d, want the unrelated 1", n)
	}
}
