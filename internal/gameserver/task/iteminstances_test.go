package task

import (
	"context"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

func TestItemInstancesSaveFlushesAndClearsPendingItems(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 10, Kind: item.KindWeapon, Weapon: &item.WeaponDetail{}},
		{ID: 20, Kind: item.KindWeapon, Weapon: &item.WeaponDetail{}},
		{ID: 30, Kind: item.KindEtcItem, EtcItem: &item.EtcItemDetail{Type: item.EtcItemPetCollar}},
	})
	items := &itemPersistenceStub{}
	augmentations := &augmentationPersistenceStub{}
	pets := &petItemPersistenceStub{}
	instances := NewItemInstances(items, augmentations, pets, templates)

	kept := &item.Instance{
		ObjectID: 1, TemplateID: 10, OwnerID: 100, Count: 5, Location: item.LocationInventory,
		Augmentation: &item.Augmentation{Attributes: 123, SkillID: 456, SkillLevel: 7},
	}
	deletedWeapon := &item.Instance{ObjectID: 2, TemplateID: 20, Count: 0, Location: item.LocationInventory}
	deletedPetCollar := &item.Instance{ObjectID: 3, TemplateID: 30, Count: 0, Location: item.LocationInventory}
	instances.Add(kept)
	instances.Add(deletedWeapon)
	instances.Add(deletedPetCollar)

	if !instances.Contains(&item.Instance{ObjectID: kept.ObjectID}) {
		t.Fatalf("Contains() should match pending items by object id")
	}
	if err := instances.Save(context.Background()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	if got, want := items.saved, []int32{1}; !slices.Equal(got, want) {
		t.Fatalf("saved item ids = %v, want %v", got, want)
	}
	if got, want := items.deleted, []int32{2, 3}; !slices.Equal(got, want) {
		t.Fatalf("deleted item ids = %v, want %v", got, want)
	}
	if got, want := augmentations.saved, []int32{1}; !slices.Equal(got, want) {
		t.Fatalf("saved augmentation ids = %v, want %v", got, want)
	}
	if got, want := augmentations.deleted, []int32{2}; !slices.Equal(got, want) {
		t.Fatalf("deleted augmentation ids = %v, want %v", got, want)
	}
	if got, want := pets.deleted, []int32{3}; !slices.Equal(got, want) {
		t.Fatalf("deleted pet item ids = %v, want %v", got, want)
	}
	if instances.Contains(kept) {
		t.Fatalf("Save() should clear successfully flushed pending items")
	}
}

func TestItemInstancesSaveDeletesVoidItemsWithoutDeletingAugmentation(t *testing.T) {
	templates := item.NewTable([]*item.Template{{ID: 10, Kind: item.KindWeapon, Weapon: &item.WeaponDetail{}}})
	items := &itemPersistenceStub{}
	augmentations := &augmentationPersistenceStub{}
	instances := NewItemInstances(items, augmentations, nil, templates)

	instances.Add(&item.Instance{
		ObjectID: 1, TemplateID: 10, Count: 1, Location: item.LocationVoid,
		Augmentation: &item.Augmentation{Attributes: 123},
	})

	if err := instances.Save(context.Background()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	if got, want := items.deleted, []int32{1}; !slices.Equal(got, want) {
		t.Fatalf("deleted item ids = %v, want %v", got, want)
	}
	if len(augmentations.deleted) != 0 {
		t.Fatalf("void item with positive count should not delete augmentation, got %v", augmentations.deleted)
	}
}

type itemPersistenceStub struct {
	saved   []int32
	deleted []int32
}

func (s *itemPersistenceStub) Save(_ context.Context, inst *item.Instance) error {
	s.saved = append(s.saved, inst.ObjectID)
	return nil
}

func (s *itemPersistenceStub) Delete(_ context.Context, objectID int32) error {
	s.deleted = append(s.deleted, objectID)
	return nil
}

type augmentationPersistenceStub struct {
	saved   []int32
	deleted []int32
}

func (s *augmentationPersistenceStub) Save(_ context.Context, objectID int32, _ item.Augmentation) error {
	s.saved = append(s.saved, objectID)
	return nil
}

func (s *augmentationPersistenceStub) Delete(_ context.Context, objectID int32) error {
	s.deleted = append(s.deleted, objectID)
	return nil
}

type petItemPersistenceStub struct {
	deleted []int32
}

func (s *petItemPersistenceStub) DeleteByItemObjectID(_ context.Context, objectID int32) error {
	s.deleted = append(s.deleted, objectID)
	return nil
}
