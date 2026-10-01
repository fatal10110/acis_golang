package itemcontainer

import (
	"errors"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

func TestInventory_UpdateWeightAndValidateWeight(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindEtcItem, Stackable: true, Weight: 10, EtcItem: &item.EtcItemDetail{}},
	})
	inv := NewPlayerInventory(0x10000001, templates)
	inv.AddNew(1, 5, 0x20000001)

	if !inv.UpdateWeight() {
		t.Fatalf("UpdateWeight() should report a change on first computation")
	}
	if inv.TotalWeight() != 50 {
		t.Errorf("TotalWeight() = %d, want 50", inv.TotalWeight())
	}
	if inv.UpdateWeight() {
		t.Errorf("UpdateWeight() should report no change when weight is unchanged")
	}

	inv.WeightLimit = 100
	if !inv.ValidateWeight(40) {
		t.Errorf("ValidateWeight(40) should fit under limit 100 with 50 already carried")
	}
	if inv.ValidateWeight(60) {
		t.Errorf("ValidateWeight(60) should exceed limit 100 with 50 already carried")
	}
}

func TestInventory_DrainUpdates_CoalescesStackableCounts(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
	})
	inv := NewPlayerInventory(0x10000001, templates)

	inst := inv.AddNew(1, 1, 0x20000001) // fresh add -> one ADDED entry
	inv.AddNew(1, 1, 0x20000002)         // merges -> one MODIFIED entry
	inv.AddNew(1, 1, 0x20000003)         // merges again -> coalesces into the same MODIFIED entry

	// ADDED and MODIFIED are tracked as distinct notifications (matching
	// the Java reference's own dedup key), so the first add's ADDED entry
	// stays separate from the two merges' single coalesced MODIFIED entry.
	updates := inv.DrainUpdates()
	if len(updates) != 2 {
		t.Fatalf("DrainUpdates() = %d entries, want 2 (one ADDED, one coalesced MODIFIED), got %+v", len(updates), updates)
	}
	if updates[0].State != UpdateAdded || updates[0].Count != 1 {
		t.Errorf("first update = %+v, want State=Added Count=1", updates[0])
	}
	if updates[1].State != UpdateModified || updates[1].Count != inst.Count {
		t.Errorf("second update = %+v, want State=Modified Count=%d", updates[1], inst.Count)
	}
	if remaining := inv.DrainUpdates(); len(remaining) != 0 {
		t.Errorf("DrainUpdates() should clear the queue, got %+v", remaining)
	}
}

// TestInventory_BuildAndDrainUpdates_KeepsQueueOnBuildError guards against
// silently losing queued deltas when the frame build fails (e.g. an item
// whose template isn't loaded): draining before build succeeds would throw
// the pending updates away with nothing sent and no way to retry them.
// The requeue must also re-register the inventory with its delivery: the
// batching task may have seen the drained, empty queue and dropped it.
func TestInventory_BuildAndDrainUpdates_KeepsQueueOnBuildError(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
	})
	delivery := &inventoryDeliveryRecorder{}
	inv := NewPetInventoryWithDelivery(1, templates, delivery, nil)
	inv.AddNew(1, 1, 1)
	if !inv.HasUpdates() {
		t.Fatal("expected AddNew to queue an update")
	}
	delivery.updates = 0

	buildErr := errors.New("boom")
	err := inv.BuildAndDrainUpdates(func(items []*item.Instance) error {
		return buildErr
	})
	if !errors.Is(err, buildErr) {
		t.Fatalf("BuildAndDrainUpdates() error = %v, want %v", err, buildErr)
	}
	if !inv.HasUpdates() {
		t.Fatal("BuildAndDrainUpdates() drained the queue despite a failed build")
	}
	if delivery.updates != 1 {
		t.Fatalf("update deliveries after a failed build = %d, want 1", delivery.updates)
	}
}

// TestInventory_BuildAndDrainUpdates_BuildRunsUnlocked pins that build runs
// with no inventory lock held: a build that changes inv must not block, and
// the update it queues lands after the drain, so the next tick delivers it.
// A failed build then puts the drained update back ahead of it.
func TestInventory_BuildAndDrainUpdates_BuildRunsUnlocked(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
		{ID: 2, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
	})
	inv := NewPetInventory(1, templates)
	inv.AddNew(1, 1, 1)

	done := make(chan error, 1)
	go func() {
		done <- inv.BuildAndDrainUpdates(func(items []*item.Instance) error {
			if len(items) != 1 {
				t.Errorf("build got %d items, want 1", len(items))
			}
			inv.AddNew(2, 1, 2)
			return nil
		})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("BuildAndDrainUpdates() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("BuildAndDrainUpdates() blocked while build changed the inventory")
	}
	if got := inv.DrainUpdates(); len(got) != 1 || got[0].ObjectID != 2 {
		t.Fatalf("queue after build = %+v, want only the update build queued", got)
	}

	inv.AddNew(1, 5, 1) // stacks onto object 1
	buildErr := errors.New("boom")
	err := inv.BuildAndDrainUpdates(func([]*item.Instance) error {
		inv.AddNew(2, 1, 2)
		return buildErr
	})
	if !errors.Is(err, buildErr) {
		t.Fatalf("BuildAndDrainUpdates() error = %v, want %v", err, buildErr)
	}
	got := inv.DrainUpdates()
	if len(got) != 2 || got[0].ObjectID != 1 || got[1].ObjectID != 2 {
		t.Fatalf("queue after failed build = %+v, want object 1 then object 2", got)
	}
}

func TestInventory_SlotsNeededFor(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
		{ID: 2, Kind: item.KindEtcItem, EtcItem: &item.EtcItemDetail{Type: item.EtcItemHerb}},
		{ID: 3, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{}},
	})
	inv := NewPlayerInventory(0x10000001, templates)

	stackTmpl, _ := templates.Get(1)
	stackInst := inv.AddNew(1, 1, 0x20000001)
	if got := inv.SlotsNeededFor(stackInst, stackTmpl); got != 0 {
		t.Errorf("SlotsNeededFor() merging into an existing stack = %d, want 0", got)
	}

	herbTmpl, _ := templates.Get(2)
	herbInst := &item.Instance{TemplateID: 2, Count: 1}
	if got := inv.SlotsNeededFor(herbInst, herbTmpl); got != 0 {
		t.Errorf("SlotsNeededFor() for a herb = %d, want 0", got)
	}

	weaponTmpl, _ := templates.Get(3)
	weaponInst := &item.Instance{TemplateID: 3, Count: 1}
	if got := inv.SlotsNeededFor(weaponInst, weaponTmpl); got != 1 {
		t.Errorf("SlotsNeededFor() for a brand new non-stackable item = %d, want 1", got)
	}
}

// TestInventory_SlotsNeededForItemIDAtSlotLimit covers every branch of the
// by-template-id slot count auto-loot relies on, and the capacity verdict for
// each at a full inventory (and with one slot free): a held stackable merges
// into its stack for 0 slots and always fits, a new stackable needs 1, a
// non-stackable needs one per unit, and an unknown template counts as
// non-stackable.
func TestInventory_SlotsNeededForItemIDAtSlotLimit(t *testing.T) {
	const (
		heldStackID int32 = 1
		newStackID  int32 = 2
		weaponID    int32 = 3
		unknownID   int32 = 99
	)
	templates := item.NewTable([]*item.Template{
		{ID: heldStackID, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
		{ID: newStackID, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
		{ID: weaponID, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{}},
	})

	tests := []struct {
		name       string
		templateID int32
		count      int
		wantSlots  int
		fitsFull   bool // SlotLimit == Size()
		fitsOneGap bool // SlotLimit == Size()+1
	}{
		{"held stackable merges", heldStackID, 5, 0, true, true},
		{"new stackable opens a stack", newStackID, 5, 1, false, true},
		{"non-stackable x1", weaponID, 1, 1, false, true},
		{"non-stackable x3", weaponID, 3, 3, false, false},
		{"unknown template counts per unit", unknownID, 2, 2, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv := NewPlayerInventory(0x10000001, templates)
			inv.AddNew(heldStackID, 10, 0x20000001)
			inv.AddNew(weaponID, 1, 0x20000002)

			if got := inv.SlotsNeededForItemID(tt.templateID, tt.count); got != tt.wantSlots {
				t.Fatalf("SlotsNeededForItemID(%d, %d) = %d, want %d", tt.templateID, tt.count, got, tt.wantSlots)
			}

			inv.SlotLimit = inv.Size()
			if got := inv.ValidateCapacityByItemID(tt.templateID, tt.count); got != tt.fitsFull {
				t.Errorf("ValidateCapacityByItemID(%d, %d) at SlotLimit == Size() = %v, want %v", tt.templateID, tt.count, got, tt.fitsFull)
			}

			inv.SlotLimit = inv.Size() + 1
			if got := inv.ValidateCapacityByItemID(tt.templateID, tt.count); got != tt.fitsOneGap {
				t.Errorf("ValidateCapacityByItemID(%d, %d) with one free slot = %v, want %v", tt.templateID, tt.count, got, tt.fitsOneGap)
			}
		})
	}
}

// TestInventory_UpdateNotifierFiresOnQueuedUpdate pins the hook every queued
// inventory change relies on, matching the reference's Inventory.addUpdate
// registering with InventoryUpdateTaskManager unconditionally: the batching
// task, not the mutation's caller, decides whether and when to drain it.
func TestInventory_UpdateNotifierFiresOnQueuedUpdate(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
	})
	delivery := &inventoryDeliveryRecorder{}
	inv := NewPlayerInventoryWithDelivery(0x10000001, templates, delivery, nil)

	inv.AddNew(1, 5, 0x30000001)
	if delivery.updates != 1 {
		t.Fatalf("update deliveries after AddNew = %d, want 1", delivery.updates)
	}

	// A coalesced update still has to register the inventory: the batch it
	// merges into may already have been drained.
	inv.AddNew(1, 5, 0x30000002)
	if delivery.updates != 2 {
		t.Errorf("update deliveries after a coalesced add = %d, want 2", delivery.updates)
	}
}

func TestInventory_DeliveryReceivesQueuedUpdatesAndWeightChanges(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindEtcItem, Stackable: true, Weight: 3, EtcItem: &item.EtcItemDetail{}},
	})
	delivery := &inventoryDeliveryRecorder{}
	inv := NewPlayerInventoryWithDelivery(0x10000001, templates, delivery, nil)

	inv.AddNew(1, 5, 0x30000001)
	if delivery.updates != 1 {
		t.Fatalf("queued update deliveries = %d, want 1", delivery.updates)
	}
	if !inv.UpdateWeight() {
		t.Fatal("UpdateWeight() = false, want true after an added item")
	}
	if delivery.weights != 1 {
		t.Fatalf("weight deliveries = %d, want 1", delivery.weights)
	}
}

// TestInventory_UpdateNotifierSkipsNoOpMutation matches the reference's
// addUpdate, which returns before registering with the manager whenever it
// appends nothing. A mutation that queues no update — an empty paperdoll
// slot here — must not fire the notifier either, or it parks the inventory
// in the batching task with nothing to send.
func TestInventory_UpdateNotifierSkipsNoOpMutation(t *testing.T) {
	templates := item.NewTable(nil)
	delivery := &inventoryDeliveryRecorder{}
	inv := NewPlayerInventoryWithDelivery(0x10000001, templates, delivery, nil)

	inv.UnequipSlot(RHand)
	if delivery.updates != 0 {
		t.Fatalf("update deliveries after unequipping an empty slot = %d, want 0", delivery.updates)
	}
}

// paperdollReadingLimiter is a slot and weight limit that reads its
// inventory's paperdoll, as an owner's stat conditions may.
type paperdollReadingLimiter struct {
	inv    *Inventory
	limit  int
	weight int
}

func (l paperdollReadingLimiter) InventoryLimit() int {
	return l.limit + len(l.inv.PaperdollItems())
}

func (l paperdollReadingLimiter) WeightLimit() int {
	return l.weight + len(l.inv.PaperdollItems())
}

// TestInventoryLimiterReplacesSlotLimit pins the owner-driven limits: a
// limiter overrides SlotLimit and WeightLimit, is asked on every check, and
// is read before Exchange locks the inventories, so a limit that reads the
// paperdoll cannot deadlock the settle check.
func TestInventoryLimiterReplacesSlotLimit(t *testing.T) {
	templates := equipTestTemplates()
	a := NewPlayerInventory(0x10000001, templates)
	b := NewPlayerInventory(0x10000002, templates)
	a.SlotLimit = 100
	a.WeightLimit = 1000
	limiter := &paperdollReadingLimiter{inv: a, limit: 1, weight: 5}
	a.SetLimiter(limiter)
	a.AddNew(swordID, 1, 0x20000001)
	if !a.ValidateWeight(5) || a.ValidateWeight(6) {
		t.Fatal("ValidateWeight(5), (6) under the limiter's weight 5 = want [true false] (WeightLimit must not apply)")
	}

	if a.ValidateCapacity(1) {
		t.Fatal("ValidateCapacity(1) at the limiter's limit = true, want false (SlotLimit must not apply)")
	}
	limiter.limit = 2
	if !a.ValidateCapacity(1) {
		t.Fatal("ValidateCapacity(1) after the limit grew = false, want true")
	}

	done := make(chan [4]bool, 1)
	go Exchange(a, b, func(heldA, heldB Held) {
		done <- [4]bool{heldA.ValidateCapacity(1), heldA.ValidateCapacity(2), heldA.ValidateWeight(5), heldA.ValidateWeight(6)}
	})
	select {
	case got := <-done:
		if got != [4]bool{true, false, true, false} {
			t.Fatalf("Held.ValidateCapacity(1), (2), ValidateWeight(5), (6) = %v, want [true false true false]", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Exchange deadlocked reading a paperdoll-reading slot limit")
	}

	a.SetLimiter(nil)
	if !a.ValidateCapacity(99) || a.ValidateCapacity(100) {
		t.Fatal("clearing the limiter did not restore SlotLimit")
	}
	if !a.ValidateWeight(1000) || a.ValidateWeight(1001) {
		t.Fatal("clearing the limiter did not restore WeightLimit")
	}
}
