package sql

import (
	"context"
	"slices"
	"strings"
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

// TestBufferSchemeStoreSaveSkipsRefusedRow pins the save's per-row
// handling: a row the table refuses is reported and left out, and the rest
// of the save still replaces what was stored before.
func TestBufferSchemeStoreSaveSkipsRefusedRow(t *testing.T) {
	ctx := context.Background()
	store := NewBufferSchemeStore(sqltest.SharedDB(t))
	if err := store.Save(ctx, []schemebuffer.Row{{OwnerID: 1, Name: "old", Skills: "1035"}}); err != nil {
		t.Fatal(err)
	}
	err := store.Save(ctx, []schemebuffer.Row{
		{OwnerID: 2, Name: "a", Skills: "1"},
		{OwnerID: 2, Name: "a", Skills: "2"},
		{OwnerID: 3, Name: "b", Skills: "3"},
	})
	if err == nil || !strings.Contains(err.Error(), `skip buffer scheme "a" of 2`) {
		t.Fatalf("Save() with a refused row = %v; want the refused row reported", err)
	}
	want := []schemebuffer.Row{{OwnerID: 2, Name: "a", Skills: "1"}, {OwnerID: 3, Name: "b", Skills: "3"}}
	if got, err := store.Load(ctx); err != nil || !slices.Equal(got, want) {
		t.Fatalf("Load() after a save with a refused row = %+v, %v; want %+v", got, err, want)
	}
}

// TestBufferSchemeStoreCancelledSaveKeepsStoredSchemes pins the save's single
// transaction: a save that cannot finish leaves the schemes stored before.
func TestBufferSchemeStoreCancelledSaveKeepsStoredSchemes(t *testing.T) {
	ctx := context.Background()
	store := NewBufferSchemeStore(sqltest.SharedDB(t))
	kept := []schemebuffer.Row{{OwnerID: 1, Name: "kept", Skills: "1035"}}
	if err := store.Save(ctx, kept); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := store.Save(cancelled, []schemebuffer.Row{{OwnerID: 2, Name: "a", Skills: "1"}}); err == nil {
		t.Fatal("Save() with a cancelled context succeeded")
	}
	if got, err := store.Load(ctx); err != nil || !slices.Equal(got, kept) {
		t.Fatalf("Load() after a failed save = %+v, %v; want %+v", got, err, kept)
	}
}
