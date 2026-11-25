package sql

import (
	"context"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/schemebuffer"
)

// TestBufferSchemeStoreSaveOwner pins the one-player write: it replaces
// every stored scheme of that player, leaves every other player's as
// stored, and with no rows removes the player's schemes.
func TestBufferSchemeStoreSaveOwner(t *testing.T) {
	ctx := context.Background()
	store := NewBufferSchemeStore(sqltest.SharedDB(t))
	other := schemebuffer.Row{OwnerID: 0x10000002, Name: "other", Skills: "1040"}
	if err := store.Save(ctx, []schemebuffer.Row{
		{OwnerID: 0x10000001, Name: "old", Skills: "1035"},
		{OwnerID: 0x10000001, Name: "tank", Skills: "1036"},
		other,
	}); err != nil {
		t.Fatal(err)
	}

	mine := []schemebuffer.Row{
		{OwnerID: 0x10000001, Name: "fight", Skills: ""},
		{OwnerID: 0x10000001, Name: "tank", Skills: "1036,1068"},
	}
	if err := store.SaveOwner(ctx, 0x10000001, mine); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Load(ctx); err != nil || !slices.Equal(got, append(slices.Clone(mine), other)) {
		t.Fatalf("Load() after SaveOwner = %+v, %v; want %+v then %+v", got, err, mine, other)
	}

	if err := store.SaveOwner(ctx, 0x10000001, nil); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Load(ctx); err != nil || !slices.Equal(got, []schemebuffer.Row{other}) {
		t.Fatalf("Load() after an empty SaveOwner = %+v, %v; want only %+v", got, err, other)
	}
}
