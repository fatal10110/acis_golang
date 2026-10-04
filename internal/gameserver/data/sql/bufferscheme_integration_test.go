package sql

import (
	"context"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/schemebuffer"
)

// TestBufferSchemeStoreRoundTrip pins buffer_schemes: an empty table loads
// nothing, a save replaces every stored scheme, rows load ordered by
// player then name, and an empty skill list is stored as "".
func TestBufferSchemeStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := NewBufferSchemeStore(sqltest.SharedDB(t))

	if got, err := store.Load(ctx); err != nil || len(got) != 0 {
		t.Fatalf("Load() on an empty table = %v, %v; want none", got, err)
	}
	if err := store.Save(ctx, []schemebuffer.Row{{OwnerID: 7, Name: "gone", Skills: "1035"}}); err != nil {
		t.Fatal(err)
	}
	saved := []schemebuffer.Row{
		{OwnerID: 0x10000002, Name: "mage", Skills: "1035,1059"},
		{OwnerID: 0x10000001, Name: "tank", Skills: "1040"},
		{OwnerID: 0x10000001, Name: "empty", Skills: ""},
	}
	if err := store.Save(ctx, saved); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []schemebuffer.Row{saved[2], saved[1], saved[0]}
	if !slices.Equal(got, want) {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

// TestBufferSchemeStoreFailedSaveKeepsStoredSchemes pins the save's single
// transaction: a row the table refuses leaves the schemes stored before.
func TestBufferSchemeStoreFailedSaveKeepsStoredSchemes(t *testing.T) {
	ctx := context.Background()
	store := NewBufferSchemeStore(sqltest.SharedDB(t))
	kept := []schemebuffer.Row{{OwnerID: 1, Name: "kept", Skills: "1035"}}
	if err := store.Save(ctx, kept); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, []schemebuffer.Row{{OwnerID: 2, Name: "a", Skills: "1"}, {OwnerID: 2, Name: "a", Skills: "2"}}); err == nil {
		t.Fatal("Save() of a duplicate key succeeded")
	}
	if got, err := store.Load(ctx); err != nil || !slices.Equal(got, kept) {
		t.Fatalf("Load() after a failed save = %+v, %v; want %+v", got, err, kept)
	}
}
