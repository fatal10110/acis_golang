package task

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

func TestItemInstancesSaveFlushesAndClearsPendingItems(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 10, Kind: item.KindWeapon, Weapon: &item.WeaponDetail{}},
		{ID: 20, Kind: item.KindWeapon, Weapon: &item.WeaponDetail{}},
		{ID: 30, Kind: item.KindEtcItem, EtcItem: &item.EtcItemDetail{Type: item.EtcItemPetCollar}},
	})
	flusher := &itemFlusherStub{}
	instances := NewItemInstances(flusher, templates)

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

	batch := flusher.last()
	if got, want := savedIDs(batch.Saves), []int32{1}; !slices.Equal(got, want) {
		t.Fatalf("saved item ids = %v, want %v", got, want)
	}
	if got, want := batch.Deletes, []int32{2, 3}; !slices.Equal(got, want) {
		t.Fatalf("deleted item ids = %v, want %v", got, want)
	}
	if got, want := augmentationSaveIDs(batch.AugmentationSaves), []int32{1}; !slices.Equal(got, want) {
		t.Fatalf("saved augmentation ids = %v, want %v", got, want)
	}
	if got, want := batch.AugmentationDeletes, []int32{2}; !slices.Equal(got, want) {
		t.Fatalf("deleted augmentation ids = %v, want %v", got, want)
	}
	if got, want := batch.PetDeletes, []int32{3}; !slices.Equal(got, want) {
		t.Fatalf("deleted pet item ids = %v, want %v", got, want)
	}
	if instances.Contains(kept) {
		t.Fatalf("Save() should clear successfully flushed pending items")
	}
}

func TestItemInstancesSaveDeletesVoidItemsWithoutDeletingAugmentation(t *testing.T) {
	templates := item.NewTable([]*item.Template{{ID: 10, Kind: item.KindWeapon, Weapon: &item.WeaponDetail{}}})
	flusher := &itemFlusherStub{}
	instances := NewItemInstances(flusher, templates)

	instances.Add(&item.Instance{
		ObjectID: 1, TemplateID: 10, Count: 1, Location: item.LocationVoid,
		Augmentation: &item.Augmentation{Attributes: 123},
	})

	if err := instances.Save(context.Background()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	batch := flusher.last()
	if got, want := batch.Deletes, []int32{1}; !slices.Equal(got, want) {
		t.Fatalf("deleted item ids = %v, want %v", got, want)
	}
	if len(batch.AugmentationDeletes) != 0 {
		t.Fatalf("void item with positive count should not delete augmentation, got %v", batch.AugmentationDeletes)
	}
}

func TestItemInstanceBackgroundAndInventoryMutationIsRaceFree(t *testing.T) {
	tmpl := &item.Template{ID: 10, Kind: item.KindEtcItem, Stackable: true, Duration: 100000, EtcItem: &item.EtcItemDetail{}}
	templates := item.NewTable([]*item.Template{tmpl})
	inv := itemcontainer.NewPlayerInventory(100, templates)
	inst := inv.AddNew(tmpl.ID, 100000, 1)

	effects := &shadowItemFakeEffects{}
	shadowItems, err := NewShadowItems(effects)
	if err != nil {
		t.Fatalf("NewShadowItems() error = %v", err)
	}
	shadowItems.Track(100, inst, tmpl)

	instances := NewItemInstances(&itemFlusherStub{}, templates)

	const iterations = 1000
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			shadowItems.Tick()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			instances.Add(inst)
			if err := instances.Save(context.Background()); err != nil {
				t.Errorf("Save() error = %v", err)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			if inv.DestroyItem(inst, 1) == nil {
				t.Errorf("DestroyItem() returned nil")
			}
		}
	}()
	wg.Wait()
}

type itemFlusherStub struct {
	mu    sync.Mutex
	batch item.FlushBatch
}

// Flush reads every save's mutable fields directly (not through
// Snapshot()), the same way a real store's Flush does, so a race between
// this and a concurrent mutation of the live instance still trips -race:
// FlushBatch.Saves is meant to hold already-detached copies, and this is
// the assertion that they actually are.
func (s *itemFlusherStub) Flush(_ context.Context, batch item.FlushBatch) error {
	for _, inst := range batch.Saves {
		_, _, _ = inst.Count, inst.Location, inst.ManaLeft
	}
	s.mu.Lock()
	s.batch = batch
	s.mu.Unlock()
	return nil
}

func (s *itemFlusherStub) last() item.FlushBatch {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.batch
}

func savedIDs(saves []item.InstanceState) []int32 {
	ids := make([]int32, len(saves))
	for i, inst := range saves {
		ids[i] = inst.ObjectID
	}
	return ids
}

func augmentationSaveIDs(saves []item.FlushAugmentationSave) []int32 {
	ids := make([]int32, len(saves))
	for i, save := range saves {
		ids[i] = save.ObjectID
	}
	return ids
}
