//go:build integration

package sql

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

func TestItemStore_CreateAndListByOwner(t *testing.T) {
	ctx := context.Background()
	store := NewItemStore(sqltest.NewDB(t))

	chest := item.Instance{
		ObjectID: 0x10000101, TemplateID: 1146, Count: 1,
		Location: item.LocationPaperdoll, LocationData: 10, ManaLeft: -1,
	}
	dagger := item.Instance{
		ObjectID: 0x10000102, TemplateID: 10, Count: 1,
		Location: item.LocationInventory, ManaLeft: -1,
	}

	if err := store.Create(ctx, 0x10000001, chest); err != nil {
		t.Fatalf("Create(chest) unexpected error: %v", err)
	}
	if err := store.Create(ctx, 0x10000001, dagger); err != nil {
		t.Fatalf("Create(dagger) unexpected error: %v", err)
	}

	got, err := store.ListByOwner(ctx, 0x10000001)
	if err != nil {
		t.Fatalf("ListByOwner() unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListByOwner() returned %d items, want 2", len(got))
	}

	byID := map[int32]*item.Instance{}
	for _, inst := range got {
		byID[inst.ObjectID] = inst
	}

	if inst := byID[chest.ObjectID]; inst == nil || inst.Location != item.LocationPaperdoll || inst.LocationData != 10 || inst.OwnerID != 0x10000001 {
		t.Errorf("chest instance = %+v, want Location=PAPERDOLL LocationData=10 OwnerID=0x10000001", inst)
	}
	if inst := byID[dagger.ObjectID]; inst == nil || inst.Location != item.LocationInventory {
		t.Errorf("dagger instance = %+v, want Location=INVENTORY", inst)
	}
}

func TestItemStore_ListByOwner_Empty(t *testing.T) {
	ctx := context.Background()
	store := NewItemStore(sqltest.NewDB(t))

	got, err := store.ListByOwner(ctx, 0x10000999)
	if err != nil {
		t.Fatalf("ListByOwner() unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ListByOwner() = %v, want empty", got)
	}
}

func TestItemStore_DeleteByOwner(t *testing.T) {
	ctx := context.Background()
	store := NewItemStore(sqltest.NewDB(t))

	for _, inst := range []item.Instance{
		{ObjectID: 0x10000101, TemplateID: 1146, Count: 1, Location: item.LocationInventory, ManaLeft: -1},
		{ObjectID: 0x10000102, TemplateID: 10, Count: 1, Location: item.LocationInventory, ManaLeft: -1},
	} {
		if err := store.Create(ctx, 0x10000001, inst); err != nil {
			t.Fatalf("Create() unexpected error: %v", err)
		}
	}

	n, err := store.DeleteByOwner(ctx, 0x10000001)
	if err != nil {
		t.Fatalf("DeleteByOwner() unexpected error: %v", err)
	}
	if n != 2 {
		t.Errorf("DeleteByOwner() deleted %d rows, want 2", n)
	}

	got, err := store.ListByOwner(ctx, 0x10000001)
	if err != nil {
		t.Fatalf("ListByOwner() after delete unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListByOwner() after delete = %v, want empty", got)
	}
}
