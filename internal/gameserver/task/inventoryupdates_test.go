package task

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

func TestInventoryUpdatesTickSendsVisibleOwnersAndUpdatesWeight(t *testing.T) {
	templates := item.NewTable([]*item.Template{{ID: 57, Kind: item.KindEtcItem, Weight: 2, Stackable: true}})
	inv := itemcontainer.NewPlayerInventory(0x10000001, templates)
	inv.Add(&item.Instance{ObjectID: 1, TemplateID: 57, Count: 3})

	owner := &inventoryUpdateOwnerStub{visible: true}
	updates := NewInventoryUpdates()
	updates.Add(inv, owner)

	updates.Tick()

	if got, want := owner.sent, [][]itemcontainer.Update{{{ObjectID: 1, TemplateID: 57, Count: 3, State: itemcontainer.UpdateAdded}}}; !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("sent updates = %+v, want %+v", got, want)
	}
	if got := inv.TotalWeight(); got != 6 {
		t.Fatalf("TotalWeight() = %d, want 6", got)
	}
	if got := inv.DrainUpdates(); len(got) != 0 {
		t.Fatalf("DrainUpdates() after send = %+v, want empty", got)
	}
}

// TestInventoryUpdatesTickBatchesMultipleMutationsIntoOneSend pins the
// batching half of the task: several mutations queued before a tick runs —
// here, two different items changing on the same inventory — reach the
// owner as exactly one SendInventoryUpdate call carrying both, not one call
// per mutation.
func TestInventoryUpdatesTickBatchesMultipleMutationsIntoOneSend(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 57, Kind: item.KindEtcItem, Weight: 2, Stackable: true},
		{ID: 58, Kind: item.KindEtcItem, Weight: 1, Stackable: true},
	})
	inv := itemcontainer.NewPlayerInventory(0x10000001, templates)

	owner := &inventoryUpdateOwnerStub{visible: true}
	updates := NewInventoryUpdates()
	inv.SetUpdateNotifier(func() { updates.Add(inv, owner) })

	inv.Add(&item.Instance{ObjectID: 1, TemplateID: 57, Count: 3})
	inv.Add(&item.Instance{ObjectID: 2, TemplateID: 58, Count: 1})

	updates.Tick()

	if len(owner.sent) != 1 {
		t.Fatalf("SendInventoryUpdate calls = %d, want 1 (one tick, one batch)", len(owner.sent))
	}
	if got := len(owner.sent[0]); got != 2 {
		t.Fatalf("updates in the single batch = %d, want 2", got)
	}
}

// TestInventoryUpdatesRemoveUnchangedKeepsReregisteredInventory pins the
// epoch guard against Tick's check-then-remove race: a mutation that lands
// between a tick observing an inventory as empty (or gated out) and the
// sweep that drops it must not orphan the update it just queued. Add bumps
// the inventory's epoch on every registration; removeUnchanged only drops
// an entry whose epoch still matches what the tick observed when it decided
// to remove it.
func TestInventoryUpdatesRemoveUnchangedKeepsReregisteredInventory(t *testing.T) {
	templates := item.NewTable([]*item.Template{{ID: 57, Kind: item.KindEtcItem, Stackable: true}})
	inv := itemcontainer.NewPlayerInventory(0x10000001, templates)
	owner := &inventoryUpdateOwnerStub{visible: true}
	updates := NewInventoryUpdates()

	updates.Add(inv, owner)
	seenEpoch := updates.epoch[inv]

	// A mutation "landing mid-tick" re-registers the inventory, bumping its
	// epoch past what this tick's snapshot observed.
	updates.Add(inv, owner)

	updates.removeUnchanged(map[*itemcontainer.Inventory]uint64{inv: seenEpoch})

	if !updates.Contains(inv) {
		t.Fatal("inventory re-registered mid-tick was dropped by the stale removal sweep")
	}

	// The ordinary case still removes: no re-registration happened, so the
	// epoch removeUnchanged sees still matches.
	updates.removeUnchanged(map[*itemcontainer.Inventory]uint64{inv: updates.epoch[inv]})
	if updates.Contains(inv) {
		t.Fatal("inventory with an unchanged epoch should have been removed")
	}
}

// TestInventoryUpdatesRemoveUnchangedClearsDroppedSlots pins that the
// in-place filter in removeUnchanged doesn't leak dropped *Inventory
// pointers past order's new length: order only ever appends and filters, so
// a leaked pointer there would keep a logged-out player's inventory (and
// every item it holds) reachable indefinitely.
func TestInventoryUpdatesRemoveUnchangedClearsDroppedSlots(t *testing.T) {
	templates := item.NewTable([]*item.Template{{ID: 57, Kind: item.KindEtcItem, Stackable: true}})
	invA := itemcontainer.NewPlayerInventory(0x10000001, templates)
	invB := itemcontainer.NewPlayerInventory(0x10000002, templates)
	owner := &inventoryUpdateOwnerStub{visible: true}
	updates := NewInventoryUpdates()

	updates.Add(invA, owner)
	updates.Add(invB, owner)

	updates.removeUnchanged(map[*itemcontainer.Inventory]uint64{invB: updates.epoch[invB]})

	full := updates.order[:cap(updates.order)]
	for i := len(updates.order); i < len(full); i++ {
		if full[i] != nil {
			t.Fatalf("order's backing array at index %d still references a dropped inventory past its new length", i)
		}
	}
}

func TestInventoryUpdatesTickDropsInvisibleNonTeleportingOwners(t *testing.T) {
	templates := item.NewTable([]*item.Template{{ID: 57, Kind: item.KindEtcItem, Stackable: true}})
	inv := itemcontainer.NewPlayerInventory(0x10000001, templates)
	inv.Add(&item.Instance{ObjectID: 1, TemplateID: 57, Count: 3})

	owner := &inventoryUpdateOwnerStub{}
	updates := NewInventoryUpdates()
	updates.Add(inv, owner)

	updates.Tick()

	if len(owner.sent) != 0 {
		t.Fatalf("sent updates = %+v, want none", owner.sent)
	}
	if updates.Contains(inv) {
		t.Fatalf("invisible non-teleporting owner should be removed from the task")
	}
	if got := inv.DrainUpdates(); len(got) != 1 {
		t.Fatalf("DrainUpdates() = %+v, want the pending update to remain queued", got)
	}
}

type inventoryUpdateOwnerStub struct {
	visible     bool
	teleporting bool
	sent        [][]itemcontainer.Update
}

func (o *inventoryUpdateOwnerStub) Visible() bool { return o.visible }

func (o *inventoryUpdateOwnerStub) Teleporting() bool { return o.teleporting }

func (o *inventoryUpdateOwnerStub) SendInventoryUpdate(updates []itemcontainer.Update) {
	o.sent = append(o.sent, slices.Clone(updates))
}
