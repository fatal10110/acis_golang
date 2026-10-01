package task

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/rs/zerolog"
)

// ---- from iteminstances_test.go ----
func TestItemInstancesSaveFlushesAndClearsPendingItems(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 10, Kind: item.KindWeapon, Weapon: &item.WeaponDetail{}},
		{ID: 20, Kind: item.KindWeapon, Weapon: &item.WeaponDetail{}},
		{ID: 30, Kind: item.KindEtcItem, EtcItem: &item.EtcItemDetail{Type: item.EtcItemPetCollar}},
	})
	flusher := &itemFlusherStub{}
	instances := NewItemInstances(flusher, templates, nil, nil, zerolog.Nop())

	kept := &item.Instance{
		ObjectID: 1, TemplateID: 10, OwnerID: 100, Count: 5, Location: item.LocationInventory,
		Augmentation: &item.Augmentation{Attributes: 123, SkillID: 456, SkillLevel: 7},
	}
	// One owner, so the kept item and the weapon share an owner batch. The
	// destroyed collar gets its own batch, keyed by its object id: it deletes
	// a pets row, whose saves run on that lane. Its id sorts after the
	// owner's, so its batch is flushed second.
	deletedWeapon := &item.Instance{ObjectID: 2, TemplateID: 20, OwnerID: 100, Count: 0, Location: item.LocationInventory}
	deletedPetCollar := &item.Instance{ObjectID: 300, TemplateID: 30, OwnerID: 100, Count: 0, Location: item.LocationInventory}
	instances.Add(kept)
	instances.Add(deletedWeapon)
	instances.Add(deletedPetCollar)

	if !instances.Contains(&item.Instance{ObjectID: kept.ObjectID}) {
		t.Fatalf("Contains() should match pending items by object id")
	}
	if err := instances.Save(context.Background()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	batches := flusher.all()
	if len(batches) != 2 {
		t.Fatalf("Flush called %d times, want 2 (owner batch, then collar batch)", len(batches))
	}
	batch := batches[0]
	if got, want := savedIDs(batch.Saves), []int32{1}; !slices.Equal(got, want) {
		t.Fatalf("saved item ids = %v, want %v", got, want)
	}
	if got, want := batch.Deletes, []int32{2}; !slices.Equal(got, want) {
		t.Fatalf("deleted item ids = %v, want %v", got, want)
	}
	if got, want := augmentationSaveIDs(batch.AugmentationSaves), []int32{1}; !slices.Equal(got, want) {
		t.Fatalf("saved augmentation ids = %v, want %v", got, want)
	}
	if got, want := batch.AugmentationDeletes, []int32{2}; !slices.Equal(got, want) {
		t.Fatalf("deleted augmentation ids = %v, want %v", got, want)
	}
	if len(batch.PetDeletes) != 0 {
		t.Fatalf("owner batch deleted pet item ids %v, want none", batch.PetDeletes)
	}
	collar := batches[1]
	if got, want := collar.Deletes, []int32{300}; !slices.Equal(got, want) {
		t.Fatalf("collar batch deleted item ids = %v, want %v", got, want)
	}
	if got, want := collar.PetDeletes, []int32{300}; !slices.Equal(got, want) {
		t.Fatalf("collar batch deleted pet item ids = %v, want %v", got, want)
	}
	if instances.Contains(kept) {
		t.Fatalf("Save() should clear successfully flushed pending items")
	}
}

func TestItemInstancesSaveDeletesVoidItemsWithoutDeletingAugmentation(t *testing.T) {
	templates := item.NewTable([]*item.Template{{ID: 10, Kind: item.KindWeapon, Weapon: &item.WeaponDetail{}}})
	flusher := &itemFlusherStub{}
	instances := NewItemInstances(flusher, templates, nil, nil, zerolog.Nop())

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

func TestItemInstancesSaveKeepsConcurrentAddDuringFlush(t *testing.T) {
	inst := &item.Instance{ObjectID: 1, TemplateID: 10, Count: 1, Location: item.LocationInventory}
	flusher := newBlockingItemFlusher(nil)
	instances := NewItemInstances(flusher, item.NewTable([]*item.Template{{ID: 10}}), nil, nil, zerolog.Nop())
	instances.Add(inst)

	done := make(chan error, 1)
	go func() { done <- instances.Save(context.Background()) }()

	select {
	case <-flusher.started:
	case <-time.After(2 * time.Second):
		t.Fatal("Flush did not start")
	}
	instances.Add(inst)
	close(flusher.release)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Save() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Save blocked")
	}
	if !instances.Contains(inst) {
		t.Fatal("Add during a successful flush must stay pending")
	}
}

func TestItemInstancesSaveRestoresPendingWhenFlushErrors(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		assertSaveKeepsPendingOnFlushResult(t, errors.New("flush failed"), false)
	})
	t.Run("deadline", func(t *testing.T) {
		assertSaveKeepsPendingOnFlushResult(t, context.DeadlineExceeded, true)
	})
}

func assertSaveKeepsPendingOnFlushResult(t *testing.T, flushErr error, waitForCtx bool) {
	t.Helper()
	inst := &item.Instance{ObjectID: 1, TemplateID: 10, Count: 1, Location: item.LocationInventory}
	flusher := newBlockingItemFlusher(flushErr)
	instances := NewItemInstances(flusher, item.NewTable([]*item.Template{{ID: 10}}), nil, nil, zerolog.Nop())
	instances.Add(inst)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if waitForCtx {
		var expire context.CancelFunc
		ctx, expire = context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer expire()
	}

	done := make(chan error, 1)
	go func() { done <- instances.Save(ctx) }()

	select {
	case <-flusher.started:
	case <-time.After(2 * time.Second):
		t.Fatal("Flush did not start")
	}
	instances.Add(inst)
	if !waitForCtx {
		close(flusher.release)
	}

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Save() error = nil, want flush failure")
		}
		if waitForCtx && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Save() error = %v, want context.DeadlineExceeded", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Save blocked")
	}
	if !instances.Contains(inst) {
		t.Fatal("failed flush must keep the item pending, including a concurrent Add")
	}
}

// TestItemInstancesSaveDoesNotResurrectRemovedItemsOnFlushError mirrors
// network.GameClientLink.flushItemPersistence: a container write succeeds
// and calls RemoveItems while a separate, unrelated flush of the same
// object id is still in flight and later fails. The failed flush's
// merge-back must not put the removed item back into pending.
func TestItemInstancesSaveDoesNotResurrectRemovedItemsOnFlushError(t *testing.T) {
	inst := &item.Instance{ObjectID: 1, TemplateID: 10, Count: 1, Location: item.LocationInventory}
	flusher := newBlockingItemFlusher(errors.New("flush failed"))
	instances := NewItemInstances(flusher, item.NewTable([]*item.Template{{ID: 10}}), nil, nil, zerolog.Nop())
	instances.Add(inst)

	done := make(chan error, 1)
	go func() { done <- instances.Save(context.Background()) }()

	select {
	case <-flusher.started:
	case <-time.After(2 * time.Second):
		t.Fatal("Flush did not start")
	}

	instances.RemoveItems([]*item.Instance{inst})
	close(flusher.release)

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Save() error = nil, want flush failure")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Save blocked")
	}
	if instances.Contains(inst) {
		t.Fatal("RemoveItems during a flush must not be undone by that flush's error merge-back")
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

	instances := NewItemInstances(&itemFlusherStub{}, templates, nil, nil, zerolog.Nop())

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
	mu      sync.Mutex
	batch   item.FlushBatch
	batches []item.FlushBatch
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
	s.batches = append(s.batches, batch)
	s.mu.Unlock()
	return nil
}

func (s *itemFlusherStub) all() []item.FlushBatch {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.batches)
}

func (s *itemFlusherStub) last() item.FlushBatch {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.batch
}

type blockingItemFlusher struct {
	started chan struct{}
	release chan struct{}
	err     error
}

func newBlockingItemFlusher(err error) *blockingItemFlusher {
	return &blockingItemFlusher{
		started: make(chan struct{}),
		release: make(chan struct{}),
		err:     err,
	}
}

func (f *blockingItemFlusher) Flush(ctx context.Context, _ item.FlushBatch) error {
	close(f.started)
	select {
	case <-f.release:
		return f.err
	case <-ctx.Done():
		return ctx.Err()
	}
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
