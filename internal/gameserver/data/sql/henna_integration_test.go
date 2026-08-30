package sql

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
)

func TestHennaStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	store := NewHennaStore(db)

	const owner int32 = 0x10000011
	if err := store.Insert(ctx, owner, 1, 1); err != nil {
		t.Fatalf("Insert first: %v", err)
	}
	if err := store.Insert(ctx, owner, 2, 2); err != nil {
		t.Fatalf("Insert second: %v", err)
	}
	got, err := store.ListByOwner(ctx, owner)
	if err != nil {
		t.Fatalf("ListByOwner: %v", err)
	}
	if len(got) != 2 || got[0].Slot != 1 || got[0].SymbolID != 1 || got[1].Slot != 2 || got[1].SymbolID != 2 {
		t.Fatalf("ListByOwner = %+v", got)
	}
	if err := store.Delete(ctx, owner, 1); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err = store.ListByOwner(ctx, owner)
	if err != nil {
		t.Fatalf("ListByOwner after delete: %v", err)
	}
	if len(got) != 1 || got[0].Slot != 2 {
		t.Fatalf("ListByOwner after delete = %+v", got)
	}
	if err := store.DeleteByOwner(ctx, owner); err != nil {
		t.Fatalf("DeleteByOwner: %v", err)
	}
	got, err = store.ListByOwner(ctx, owner)
	if err != nil {
		t.Fatalf("ListByOwner after DeleteByOwner: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ListByOwner after DeleteByOwner = %+v, want empty", got)
	}
}
