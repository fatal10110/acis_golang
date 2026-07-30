package itemcontainer

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// TestContainerItemPersisterCoversAddedItems proves an item entering a
// wired container reports both the move that brought it in and every later
// mutation, so persistence follows the item rather than the call site.
func TestContainerItemPersisterCoversAddedItems(t *testing.T) {
	c := newTestContainer()

	var changed []int32
	c.SetItemPersister(func(inst *item.Instance) { changed = append(changed, inst.ObjectID) })

	inst := c.AddNew(potionTemplateID, 5, 0x20000001)
	if inst == nil {
		t.Fatal("AddNew() = nil")
	}
	if len(changed) != 1 || changed[0] != 0x20000001 {
		t.Fatalf("after AddNew, changed = %v, want [0x20000001]", changed)
	}

	// A count change made with no client involved must still be reported.
	inst.ReduceCount(2)
	if len(changed) != 2 {
		t.Fatalf("after ReduceCount, changed = %v, want two entries", changed)
	}

	// So must the destruction that removes it, since the row has to be
	// deleted rather than left behind.
	if got := c.DestroyAll(inst); got == nil {
		t.Fatal("DestroyAll() = nil")
	}
	if len(changed) != 3 {
		t.Fatalf("after DestroyAll, changed = %v, want three entries", changed)
	}
}

// TestContainerAddKeepsPersisterOfUnwiredDestination pins that moving an
// item into a container nothing has wired — a warehouse, a freight, a pet
// inventory before its owner logs in — neither swallows the move nor
// silences the item afterwards. Dropping the hook there would leave the
// items row pointing at the container the item just left.
func TestContainerAddKeepsPersisterOfUnwiredDestination(t *testing.T) {
	source := newTestContainer()
	var changed []int32
	source.SetItemPersister(func(inst *item.Instance) { changed = append(changed, inst.ObjectID) })

	inst := source.AddNew(daggerTemplateID, 1, 0x20000001)
	if inst == nil {
		t.Fatal("AddNew() = nil")
	}
	if !source.Remove(inst) {
		t.Fatal("Remove() = false")
	}
	before := len(changed)

	// A destination with no persister of its own.
	target := NewContainer(0x10000002, item.LocationWarehouse, testTemplates())
	if _, absorbed := target.Add(inst); absorbed {
		t.Fatal("Add() absorbed a non-stackable item")
	}
	if len(changed) != before+1 {
		t.Fatalf("moving into an unwired container reported %d changes, want 1", len(changed)-before)
	}

	inst.SetEnchantLevel(3)
	if len(changed) != before+2 {
		t.Errorf("mutating after the move reported %d changes, want 1", len(changed)-before-1)
	}
}

// TestContainerAddAbsorbedItemReportsDestruction covers the merge path: the
// absorbed instance's units are now counted on the pre-existing stack, so
// any row it had must be deleted rather than left behind double-counting
// them after a restart.
func TestContainerAddAbsorbedItemReportsDestruction(t *testing.T) {
	c := newTestContainer()
	c.SetItemPersister(func(*item.Instance) {})

	if first := c.AddNew(adenaTemplateID, 100, 0x20000001); first == nil {
		t.Fatal("AddNew() = nil")
	}

	// An incoming stack that already has a row of its own.
	incoming := &item.Instance{ObjectID: 0x20000002, TemplateID: adenaTemplateID, Count: 50, OwnerID: 0x10000009, Location: item.LocationInventory, ManaLeft: -1}
	var reported []item.InstanceState
	incoming.SetPersistNotifier(func(inst *item.Instance) { reported = append(reported, inst.Snapshot()) })

	result, absorbed := c.Add(incoming)
	if !absorbed {
		t.Fatal("Add() did not absorb a stackable item")
	}
	if got := result.CountValue(); got != 150 {
		t.Errorf("merged stack count = %d, want 150", got)
	}
	if len(reported) == 0 {
		t.Fatal("absorbed item reported no change; its row would survive the merge")
	}
	last := reported[len(reported)-1]
	if last.Count != 0 || last.Location != item.LocationVoid {
		t.Errorf("absorbed item reported count=%d loc=%v, want a destroyed state", last.Count, last.Location)
	}
}

// TestFreightAddAbsorbedItemReportsDestruction covers the same merge path
// through Freight's own Add.
func TestFreightAddAbsorbedItemReportsDestruction(t *testing.T) {
	f := NewFreight(0x10000001, testTemplates())
	f.SetItemPersister(func(*item.Instance) {})

	if first := f.AddNew(adenaTemplateID, 100, 0x20000001); first == nil {
		t.Fatal("AddNew() = nil")
	}

	incoming := &item.Instance{ObjectID: 0x20000002, TemplateID: adenaTemplateID, Count: 50, OwnerID: 0x10000009, Location: item.LocationInventory, ManaLeft: -1}
	destroyed := false
	incoming.SetPersistNotifier(func(inst *item.Instance) {
		if st := inst.Snapshot(); st.Count == 0 && st.Location == item.LocationVoid {
			destroyed = true
		}
	})

	if _, absorbed := f.Add(incoming); !absorbed {
		t.Fatal("Add() did not absorb a stackable item")
	}
	if !destroyed {
		t.Error("absorbed freight item never reported its destruction")
	}
}

// TestInventoryItemPersisterAppliesToRestoredItems covers the login order:
// an inventory is restored from its persisted rows first and wired to the
// persistence task afterwards. Restoring must not schedule a write of what
// was just read, but the items must be covered from then on.
func TestInventoryItemPersisterAppliesToRestoredItems(t *testing.T) {
	restored := []*item.Instance{
		{ObjectID: 0x20000001, TemplateID: potionTemplateID, Count: 5, Location: item.LocationInventory, ManaLeft: -1},
	}
	inv := RestorePlayerInventory(0x10000001, testTemplates(), restored)

	notified := 0
	inv.SetItemPersister(func(*item.Instance) { notified++ })
	if notified != 0 {
		t.Fatalf("wiring a restored inventory notified %d times, want 0", notified)
	}

	held := inv.ItemByObjectID(0x20000001)
	if held == nil {
		t.Fatal("restored item missing from inventory")
	}
	held.AddCount(1)
	if notified != 1 {
		t.Errorf("notifications after mutating a restored item = %d, want 1", notified)
	}
}

// TestInventoryItemPersisterClearedOnDetach proves the hook is releasable,
// so a logged-out player's items stop registering with the task.
func TestInventoryItemPersisterClearedOnDetach(t *testing.T) {
	inv := RestorePlayerInventory(0x10000001, testTemplates(), nil)

	notified := 0
	inv.SetItemPersister(func(*item.Instance) { notified++ })

	inst := inv.AddNew(potionTemplateID, 5, 0x20000001)
	if inst == nil {
		t.Fatal("AddNew() = nil")
	}
	before := notified

	inv.SetItemPersister(nil)
	inst.AddCount(1)
	if notified != before {
		t.Errorf("notifications after clearing = %d, want %d", notified, before)
	}
}
