package sql

import (
	"context"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/wedding"
)

// TestCoupleStoreRoundTrip pins mods_wedding: an empty table loads
// nothing, a save replaces every stored couple, couples load by id, and
// saving none empties the table.
func TestCoupleStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := NewCoupleStore(sqltest.SharedDB(t))

	if got, err := store.Load(ctx); err != nil || len(got) != 0 {
		t.Fatalf("Load() on an empty table = %v, %v; want none", got, err)
	}
	if err := store.Save(ctx, []wedding.Couple{{ID: 0x10000009, RequesterID: 1, PartnerID: 2}}); err != nil {
		t.Fatal(err)
	}
	saved := []wedding.Couple{
		{ID: 0x10000020, RequesterID: 0x10000003, PartnerID: 0x10000004},
		{ID: 0x10000010, RequesterID: 0x10000001, PartnerID: 0x10000002},
	}
	if err := store.Save(ctx, saved); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := []wedding.Couple{saved[1], saved[0]}; !slices.Equal(got, want) {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
	if err := store.Save(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Load(ctx); err != nil || len(got) != 0 {
		t.Fatalf("Load() after saving none = %v, %v; want none", got, err)
	}
}
