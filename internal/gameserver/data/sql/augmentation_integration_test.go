//go:build integration

package sql

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

func TestAugmentationStore_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	store := NewAugmentationStore(sqltest.NewDB(t))

	aug := item.Augmentation{Attributes: 12345, SkillID: 2621, SkillLevel: 1}
	if err := store.Create(ctx, 0x10000101, aug); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	got, ok, err := store.Get(ctx, 0x10000101)
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("Get() reported not found, want found")
	}
	if got != aug {
		t.Errorf("Get() = %+v, want %+v", got, aug)
	}
}

func TestAugmentationStore_Get_NotFound(t *testing.T) {
	ctx := context.Background()
	store := NewAugmentationStore(sqltest.NewDB(t))

	_, ok, err := store.Get(ctx, 0x10000999)
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if ok {
		t.Errorf("Get() on an item with no augmentation should report not found")
	}
}

func TestAugmentationStore_Delete(t *testing.T) {
	ctx := context.Background()
	store := NewAugmentationStore(sqltest.NewDB(t))

	if err := store.Create(ctx, 0x10000101, item.Augmentation{Attributes: 1}); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if err := store.Delete(ctx, 0x10000101); err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}

	_, ok, err := store.Get(ctx, 0x10000101)
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if ok {
		t.Errorf("Get() after Delete() should report not found")
	}
}
