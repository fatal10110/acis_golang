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
