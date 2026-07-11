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

func TestItemStore_Update(t *testing.T) {
	ctx := context.Background()
	store := NewItemStore(sqltest.NewDB(t))

	inst := item.Instance{
		ObjectID: 0x10000101, TemplateID: 1146, Count: 1,
		Location: item.LocationInventory, ManaLeft: -1,
	}
	if err := store.Create(ctx, 0x10000001, inst); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	inst.OwnerID = 0x10000001
	inst.Count = 5
	inst.EnchantLevel = 7
	inst.Location = item.LocationPaperdoll
	inst.LocationData = 10
	inst.ManaLeft = 42
	if err := store.Update(ctx, &inst); err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}

	got, err := store.ListByOwner(ctx, 0x10000001)
	if err != nil {
		t.Fatalf("ListByOwner() unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListByOwner() returned %d items, want 1", len(got))
	}
	if got[0].Count != 5 || got[0].EnchantLevel != 7 || got[0].Location != item.LocationPaperdoll || got[0].LocationData != 10 || got[0].ManaLeft != 42 {
		t.Errorf("updated instance = %+v, want Count=5 EnchantLevel=7 Location=PAPERDOLL LocationData=10 ManaLeft=42", got[0])
	}
}

func TestItemStore_SaveUpserts(t *testing.T) {
	ctx := context.Background()
	store := NewItemStore(sqltest.NewDB(t))

	inst := &item.Instance{
		ObjectID: 0x10000101, TemplateID: 1146, OwnerID: 0x10000001, Count: 1,
		Location: item.LocationInventory, ManaLeft: -1,
	}
	if err := store.Save(ctx, inst); err != nil {
		t.Fatalf("Save(insert) unexpected error: %v", err)
	}

	inst.Count = 5
	inst.EnchantLevel = 7
	inst.Location = item.LocationPaperdoll
	inst.LocationData = 10
	inst.ManaLeft = 42
	if err := store.Save(ctx, inst); err != nil {
		t.Fatalf("Save(update) unexpected error: %v", err)
	}

	got, err := store.ListByOwner(ctx, 0x10000001)
	if err != nil {
		t.Fatalf("ListByOwner() unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListByOwner() returned %d items, want 1", len(got))
	}
	if got[0].Count != 5 || got[0].EnchantLevel != 7 || got[0].Location != item.LocationPaperdoll || got[0].LocationData != 10 || got[0].ManaLeft != 42 {
		t.Errorf("saved instance = %+v, want updated state", got[0])
	}
}

func TestItemStore_Delete(t *testing.T) {
	ctx := context.Background()
	store := NewItemStore(sqltest.NewDB(t))

	inst := item.Instance{ObjectID: 0x10000101, TemplateID: 1146, Count: 1, Location: item.LocationInventory, ManaLeft: -1}
	if err := store.Create(ctx, 0x10000001, inst); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	if err := store.Delete(ctx, inst.ObjectID); err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}

	got, err := store.ListByOwner(ctx, 0x10000001)
	if err != nil {
		t.Fatalf("ListByOwner() unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListByOwner() after delete = %v, want empty", got)
	}
}

func TestItemStore_ListByOwnerAndLocations(t *testing.T) {
	ctx := context.Background()
	store := NewItemStore(sqltest.NewDB(t))

	for _, inst := range []item.Instance{
		{ObjectID: 0x10000101, TemplateID: 1146, Count: 1, Location: item.LocationPaperdoll, LocationData: 10, ManaLeft: -1},
		{ObjectID: 0x10000102, TemplateID: 10, Count: 1, Location: item.LocationInventory, ManaLeft: -1},
		{ObjectID: 0x10000103, TemplateID: 20, Count: 1, Location: item.LocationWarehouse, ManaLeft: -1},
	} {
		if err := store.Create(ctx, 0x10000001, inst); err != nil {
			t.Fatalf("Create() unexpected error: %v", err)
		}
	}

	got, err := store.ListByOwnerAndLocations(ctx, 0x10000001, item.LocationInventory, item.LocationPaperdoll)
	if err != nil {
		t.Fatalf("ListByOwnerAndLocations() unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListByOwnerAndLocations() returned %d items, want 2 (warehouse item excluded)", len(got))
	}
	for _, inst := range got {
		if inst.Location == item.LocationWarehouse {
			t.Errorf("ListByOwnerAndLocations() should not return a warehouse item when only INVENTORY/PAPERDOLL were requested")
		}
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
