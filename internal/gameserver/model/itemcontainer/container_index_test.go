package itemcontainer

import (
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// assertIndexed fails t unless c's template index lists exactly the
// instances c holds, each once, under its own template id, with no empty
// bucket left behind.
func assertIndexed(t *testing.T, c *Container) {
	t.Helper()
	c.mu.RLock()
	defer c.mu.RUnlock()
	seen := make(map[*item.Instance]bool, len(c.items))
	for templateID, bucket := range c.byTemplate {
		if len(bucket) == 0 {
			t.Errorf("template %d: empty index bucket kept", templateID)
		}
		for _, inst := range bucket {
			switch {
			case inst.TemplateID != templateID:
				t.Errorf("template %d: index lists object %d of template %d", templateID, inst.ObjectID, inst.TemplateID)
			case c.items[inst.ObjectID] != inst:
				t.Errorf("template %d: index lists object %d, which the container does not hold", templateID, inst.ObjectID)
			case seen[inst]:
				t.Errorf("template %d: index lists object %d twice", templateID, inst.ObjectID)
			}
			seen[inst] = true
		}
	}
	if len(seen) != len(c.items) {
		t.Errorf("index lists %d instances, container holds %d", len(seen), len(c.items))
	}
}

func assertHolds(t *testing.T, c *Container, templateID int32, want bool) {
	t.Helper()
	if got := c.HasItem(templateID); got != want {
		t.Errorf("HasItem(%d) = %v, want %v", templateID, got, want)
	}
	if got := c.ItemByTemplateID(templateID) != nil; got != want {
		t.Errorf("ItemByTemplateID(%d) found = %v, want %v", templateID, got, want)
	}
	if got := len(c.ItemsByTemplateID(templateID)) > 0; got != want {
		t.Errorf("ItemsByTemplateID(%d) non-empty = %v, want %v", templateID, got, want)
	}
	assertIndexed(t, c)
}

func objectIDs(items []*item.Instance) []int32 {
	out := make([]int32, 0, len(items))
	for _, inst := range items {
		out = append(out, inst.ObjectID)
	}
	return out
}

func TestContainerIndex_FollowsAddRemoveDestroy(t *testing.T) {
	t.Parallel()
	c := newTestContainer()

	for _, id := range []int32{0x20000001, 0x20000002, 0x20000003} {
		c.AddNew(daggerTemplateID, 1, id)
	}
	adena := c.AddNew(adenaTemplateID, 100, 0x20000010)
	if merged := c.AddNew(adenaTemplateID, 50, 0x20000011); merged != adena {
		t.Fatalf("AddNew(adena) = %v, want the held stack", merged)
	}
	potion := c.AddNew(potionTemplateID, 5, 0x20000020)
	assertHolds(t, c, adenaTemplateID, true)
	if got := len(c.ItemsByTemplateID(daggerTemplateID)); got != 3 {
		t.Fatalf("ItemsByTemplateID(dagger) = %d instances, want 3", got)
	}
	if !c.HasItems(daggerTemplateID, adenaTemplateID, potionTemplateID) || c.HasItems(daggerTemplateID, 999) {
		t.Fatal("HasItems disagrees with the held set")
	}
	if !c.HasItems() || c.HasAnyItem() || !c.HasAnyItem(999, potionTemplateID) || c.HasAnyItem(998, 999) {
		t.Fatal("HasItems/HasAnyItem disagree with the held set or the empty-id rule")
	}

	if !c.Remove(c.ItemByObjectID(0x20000002)) {
		t.Fatal("Remove(held dagger) = false")
	}
	assertHolds(t, c, daggerTemplateID, true)

	if c.DestroyItem(adena, 30) != adena {
		t.Fatal("DestroyItem(adena, 30) did not reduce the stack")
	}
	assertHolds(t, c, adenaTemplateID, true)
	if got := c.ItemCount(adenaTemplateID, -1, true); got != 120 {
		t.Fatalf("ItemCount(adena) = %d, want 120", got)
	}

	if c.DestroyItem(potion, 5) != potion {
		t.Fatal("DestroyItem(potion, all) did not destroy the stack")
	}
	assertHolds(t, c, potionTemplateID, false)

	c.DestroyByTemplateID(daggerTemplateID, 1)
	c.DestroyByTemplateID(daggerTemplateID, 1)
	assertHolds(t, c, daggerTemplateID, false)

	c.AddNew(daggerTemplateID, 1, 0x20000030)
	c.DestroyAllItems()
	assertHolds(t, c, daggerTemplateID, false)
	assertHolds(t, c, adenaTemplateID, false)
}

func TestContainerIndex_FollowsTransfer(t *testing.T) {
	t.Parallel()
	src, dst := newTestContainer(), newTestContainer()
	src.AddNew(adenaTemplateID, 100, 0x20000001)
	src.AddNew(daggerTemplateID, 1, 0x20000002)
	src.AddNew(potionTemplateID, 10, 0x20000003)
	dst.AddNew(potionTemplateID, 1, 0x20000004)
	nextID := func() (int32, error) { return 0x20000100, nil }

	// A split into a target holding none creates a new instance there.
	if res, _, _, err := src.Transfer(0x20000001, 40, dst, nextID); err != nil || res == nil {
		t.Fatalf("Transfer(adena split) = %v, %v", res, err)
	}
	// A whole non-stackable moves into the target as itself.
	if res, _, _, _ := src.Transfer(0x20000002, 1, dst, nil); res == nil || res.ObjectID != 0x20000002 {
		t.Fatalf("Transfer(dagger whole) = %v, want the instance itself", res)
	}
	// A whole stack merged into the target's stack leaves the source.
	if _, freedID, freed, _ := src.Transfer(0x20000003, 10, dst, nil); !freed || freedID != 0x20000003 {
		t.Fatalf("Transfer(potion merged) freed = %v %d, want 0x20000003", freed, freedID)
	}

	assertHolds(t, src, adenaTemplateID, true)
	assertHolds(t, src, daggerTemplateID, false)
	assertHolds(t, src, potionTemplateID, false)
	assertHolds(t, dst, adenaTemplateID, true)
	assertHolds(t, dst, daggerTemplateID, true)
	assertHolds(t, dst, potionTemplateID, true)
	if got := dst.ItemCount(potionTemplateID, -1, true); got != 11 {
		t.Fatalf("dst ItemCount(potion) = %d, want 11", got)
	}
}

func TestContainerIndex_RestoreReplacesIndex(t *testing.T) {
	t.Parallel()
	c := newTestContainer()
	row := func(objectID, templateID int32, count int) *item.Instance {
		return &item.Instance{ObjectID: objectID, TemplateID: templateID, Count: count, Location: item.LocationWarehouse}
	}

	c.Restore([]*item.Instance{
		row(0x20000001, adenaTemplateID, 10),
		row(0x20000002, adenaTemplateID, 5), // merges into the first stack
		row(0x20000003, daggerTemplateID, 1),
		{ObjectID: 0x20000004, TemplateID: potionTemplateID, Count: 1, Location: item.LocationInventory}, // not this container's
	})
	assertHolds(t, c, adenaTemplateID, true)
	assertHolds(t, c, daggerTemplateID, true)
	assertHolds(t, c, potionTemplateID, false)
	if got := objectIDs(c.ItemsByTemplateID(adenaTemplateID)); !slices.Equal(got, []int32{0x20000001}) {
		t.Fatalf("restored adena stacks = %v, want the merged first row only", got)
	}
	if got := c.ItemCount(adenaTemplateID, -1, true); got != 15 {
		t.Fatalf("ItemCount(adena) = %d, want 15", got)
	}

	c.Restore([]*item.Instance{row(0x20000005, potionTemplateID, 3)})
	assertHolds(t, c, adenaTemplateID, false)
	assertHolds(t, c, daggerTemplateID, false)
	assertHolds(t, c, potionTemplateID, true)
}

func TestInventoryIndex_RestoreAndDestroyAll(t *testing.T) {
	t.Parallel()
	inv := NewPlayerInventory(0x10000001, testTemplates())
	rhand, _ := item.SlotRHand.PaperdollIndex()
	inv.AddNew(potionTemplateID, 1, 0x20000009)
	inv.Restore([]*item.Instance{
		{ObjectID: 0x20000001, TemplateID: adenaTemplateID, Count: 10, Location: item.LocationInventory},
		{ObjectID: 0x20000002, TemplateID: adenaTemplateID, Count: 7, Location: item.LocationInventory},
		{ObjectID: 0x20000003, TemplateID: daggerTemplateID, Count: 1, Location: item.LocationPaperdoll, LocationData: rhand},
		{ObjectID: 0x20000004, TemplateID: daggerTemplateID, Count: 1, Location: item.LocationInventory},
	})
	assertHolds(t, inv.Container, potionTemplateID, false)
	assertHolds(t, inv.Container, adenaTemplateID, true)
	if got := len(inv.ItemsByTemplateID(daggerTemplateID)); got != 2 {
		t.Fatalf("ItemsByTemplateID(dagger) = %d instances, want 2", got)
	}
	if got := inv.ItemCount(adenaTemplateID, -1, true); got != 17 {
		t.Fatalf("ItemCount(adena) = %d, want 17", got)
	}
	if got := inv.ItemCount(daggerTemplateID, -1, false); got != 1 {
		t.Fatalf("ItemCount(dagger, unequipped) = %d, want 1", got)
	}

	inv.Restore([]*item.Instance{
		{ObjectID: 0x20000005, TemplateID: potionTemplateID, Count: 2, Location: item.LocationInventory},
	})
	assertHolds(t, inv.Container, adenaTemplateID, false)
	assertHolds(t, inv.Container, daggerTemplateID, false)
	assertHolds(t, inv.Container, potionTemplateID, true)

	inv.DestroyAllItems()
	assertHolds(t, inv.Container, potionTemplateID, false)
}

func TestFreightIndex_FollowsAdd(t *testing.T) {
	t.Parallel()
	f := NewFreight(0x10000001, freightTestTemplates())
	f.ActiveLocation = 1
	f.AddNew(freightTestItemID, 1, 0x20000001)
	if _, err := f.AddCreated(freightTestStackableID, 5, func() (int32, error) { return 0x20000002, nil }); err != nil {
		t.Fatalf("AddCreated() error = %v", err)
	}
	f.ActiveLocation = 2
	f.Add(&item.Instance{ObjectID: 0x20000003, TemplateID: freightTestItemID, Count: 1})
	f.AddNew(freightTestStackableID, 3, 0x20000004) // a stack at another town stays separate

	assertHolds(t, f.Container, freightTestItemID, true)
	assertHolds(t, f.Container, freightTestStackableID, true)
	if got := len(f.Container.ItemsByTemplateID(freightTestStackableID)); got != 2 {
		t.Fatalf("Container.ItemsByTemplateID(stackable) = %d instances, want one per town", got)
	}
	if got := len(f.Container.ItemsByTemplateID(freightTestItemID)); got != 2 {
		t.Fatalf("Container.ItemsByTemplateID(item) = %d instances, want 2", got)
	}
}

func TestHeldTransferIndex(t *testing.T) {
	t.Parallel()
	a := NewPlayerInventory(0x10000001, testTemplates())
	b := NewPlayerInventory(0x10000002, testTemplates())
	a.AddNew(adenaTemplateID, 100, 0x20000001)
	a.AddNew(daggerTemplateID, 1, 0x20000002)
	a.AddNew(potionTemplateID, 4, 0x20000003)
	b.AddNew(potionTemplateID, 1, 0x20000004)

	Exchange(a, b, func(ha, hb Held) {
		if _, ok := ha.Transfer(0x20000001, 30, hb, 0x20000100); !ok {
			t.Error("Transfer(adena split) refused")
		}
		if _, ok := ha.Transfer(0x20000002, 1, hb, 0); !ok {
			t.Error("Transfer(dagger whole) refused")
		}
		if m, ok := ha.Transfer(0x20000003, 4, hb, 0); !ok || m.FreedObjectID != 0x20000003 {
			t.Errorf("Transfer(potion merged) = %+v, %v", m, ok)
		}
	})

	assertHolds(t, a.Container, adenaTemplateID, true)
	assertHolds(t, a.Container, daggerTemplateID, false)
	assertHolds(t, a.Container, potionTemplateID, false)
	assertHolds(t, b.Container, adenaTemplateID, true)
	assertHolds(t, b.Container, daggerTemplateID, true)
	assertHolds(t, b.Container, potionTemplateID, true)
}

// TestContainerIndex_OrderSurvivesBucketReshuffle pins that the index never
// supplies the order: removing one instance reshuffles its template's bucket,
// and entry times change after insertion, yet the readers still answer in
// byContainerOrder.
func TestContainerIndex_OrderSurvivesBucketReshuffle(t *testing.T) {
	t.Parallel()
	c := NewWarehouse(1, testTemplates())
	for _, id := range []int32{601, 602, 603, 604, 605} {
		c.AddNew(daggerTemplateID, 1, id)
	}
	c.Remove(c.ItemByObjectID(601))
	for id, at := range map[int32]int64{602: 100, 603: 300, 604: 300, 605: 200} {
		c.ItemByObjectID(id).SetTime(at)
	}

	want := []int32{604, 603, 605, 602}
	if got := objectIDs(c.ItemsByTemplateID(daggerTemplateID)); !slices.Equal(got, want) {
		t.Fatalf("ItemsByTemplateID() = %v, want %v", got, want)
	}
	if got := c.ItemByTemplateID(daggerTemplateID); got == nil || got.ObjectID != want[0] {
		t.Fatalf("ItemByTemplateID() = %v, want %d", got, want[0])
	}
	if got := c.DestroyByTemplateID(daggerTemplateID, 1); got == nil || got.ObjectID != want[0] {
		t.Fatalf("DestroyByTemplateID() destroyed %v, want %d", got, want[0])
	}
	assertIndexed(t, c)
}

// TestContainerIndex_ConsistentUnderConcurrentMutation races adds, merges,
// destroys and transfers against the Has* readers; -race checks the readers
// stay under the container lock, and the final audit checks no mutation
// left the index out of step.
func TestContainerIndex_ConsistentUnderConcurrentMutation(t *testing.T) {
	t.Parallel()
	src, dst := newTestContainer(), newTestContainer()
	const workers, perWorker = 4, 50

	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			base := int32(0x20000000 + w*0x1000)
			for i := range int32(perWorker) {
				id := base + i*4
				src.AddNew(daggerTemplateID, 1, id)
				src.AddNew(adenaTemplateID, 10, id+1)
				switch i % 3 {
				case 0:
					src.DestroyByObjectID(id, 1)
				case 1:
					src.Transfer(id, 1, dst, nil)
				default:
					src.DestroyByTemplateID(adenaTemplateID, 5)
				}
			}
		})
		wg.Go(func() {
			for range perWorker {
				_ = src.HasItems(daggerTemplateID, adenaTemplateID)
				_ = dst.HasAnyItem(potionTemplateID, daggerTemplateID)
				_ = src.ItemsByTemplateID(daggerTemplateID)
				_ = src.ItemCount(adenaTemplateID, -1, true)
			}
		})
	}
	wg.Wait()

	assertIndexed(t, src)
	assertIndexed(t, dst)
}

// BenchmarkContainerHasItems is the hot per-hit quest check: a few template
// ids tested against a full player inventory.
func BenchmarkContainerHasItems(b *testing.B) {
	const size = 80
	templates := make([]*item.Template, 0, size)
	for i := range int32(size) {
		templates = append(templates, &item.Template{ID: 1000 + i, Kind: item.KindEtcItem, EtcItem: &item.EtcItemDetail{}})
	}
	inv := NewPlayerInventory(0x10000001, item.NewTable(templates))
	for i := range int32(size) {
		inv.AddNew(1000+i, 1, 0x20000000+i)
	}
	ids := []int32{1003, 1031, 1057, 1079, 5000}

	b.ReportAllocs()
	for b.Loop() {
		_ = inv.HasItems(ids[:4]...)
		_ = inv.HasAnyItem(ids[4], ids[0])
		_ = inv.ItemCount(ids[2], -1, true)
	}
}
