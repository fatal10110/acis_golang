package itemcontainer

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

const (
	freightTestItemID      int32 = 100
	freightTestStackableID int32 = 101
)

func freightTestTemplates() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: freightTestItemID, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{}},
		{ID: freightTestStackableID, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
	})
}

func TestFreight_AddNew_TagsCurrentTown(t *testing.T) {
	f := NewFreight(0x10000001, freightTestTemplates())

	untagged := f.AddNew(freightTestItemID, 1, 0x20000001)
	if untagged.LocationData != 0 {
		t.Errorf("AddNew() with no active town set LocationData = %d, want 0", untagged.LocationData)
	}

	f.ActiveLocation = 5
	tagged := f.AddNew(freightTestItemID, 1, 0x20000002)
	if tagged.LocationData != 5 {
		t.Errorf("AddNew() with active town 5 set LocationData = %d, want 5", tagged.LocationData)
	}
}

func TestFreight_AddNew_MergesOnlyVisibleStacks(t *testing.T) {
	f := NewFreight(0x10000001, freightTestTemplates())

	f.ActiveLocation = 1
	townOne := f.AddNew(freightTestStackableID, 10, 0x20000001)
	f.ActiveLocation = 2
	townTwo := f.AddNew(freightTestStackableID, 5, 0x20000002)

	if townTwo == townOne {
		t.Fatalf("AddNew() merged a town-2 stack into a hidden town-1 stack")
	}
	if townOne.Count != 10 || townOne.LocationData != 1 {
		t.Errorf("town-1 stack = count %d location %d, want count 10 location 1", townOne.Count, townOne.LocationData)
	}
	if townTwo.Count != 5 || townTwo.LocationData != 2 {
		t.Errorf("town-2 stack = count %d location %d, want count 5 location 2", townTwo.Count, townTwo.LocationData)
	}

	merged := f.AddNew(freightTestStackableID, 3, 0x20000003)
	if merged != townTwo {
		t.Fatalf("AddNew() with the same active town returned %+v, want the town-2 stack", merged)
	}
	if townTwo.Count != 8 || townTwo.LocationData != 2 {
		t.Errorf("merged town-2 stack = count %d location %d, want count 8 location 2", townTwo.Count, townTwo.LocationData)
	}
}

func TestFreight_AddNew_MergesUntaggedStackWithoutRetagging(t *testing.T) {
	f := NewFreight(0x10000001, freightTestTemplates())

	untagged := f.AddNew(freightTestStackableID, 10, 0x20000001)
	f.ActiveLocation = 2
	merged := f.AddNew(freightTestStackableID, 5, 0x20000002)

	if merged != untagged {
		t.Fatalf("AddNew() should merge into a visible untagged stack")
	}
	if untagged.Count != 15 || untagged.LocationData != 0 {
		t.Errorf("untagged stack = count %d location %d, want count 15 location 0", untagged.Count, untagged.LocationData)
	}
}

func TestFreight_VisibleItems_FiltersByActiveTown(t *testing.T) {
	f := NewFreight(0x10000001, freightTestTemplates())

	f.ActiveLocation = 1
	townOne := f.AddNew(freightTestItemID, 1, 0x20000001)
	f.ActiveLocation = 2
	townTwo := f.AddNew(freightTestItemID, 1, 0x20000002)
	f.ActiveLocation = 0
	untagged := f.AddNew(freightTestItemID, 1, 0x20000003)

	// Underlying container always sees every item regardless of town.
	if f.Size() != 3 {
		t.Fatalf("Size() = %d, want 3 (unfiltered)", f.Size())
	}

	f.ActiveLocation = 1
	visible := f.VisibleItems()
	if len(visible) != 2 {
		t.Fatalf("VisibleItems() with ActiveLocation=1 = %d items, want 2 (town-1 item + untagged item)", len(visible))
	}
	var sawTownOne, sawUntagged bool
	for _, inst := range visible {
		switch inst.ObjectID {
		case townOne.ObjectID:
			sawTownOne = true
		case untagged.ObjectID:
			sawUntagged = true
		case townTwo.ObjectID:
			t.Errorf("VisibleItems() with ActiveLocation=1 must not include the town-2 item")
		}
	}
	if !sawTownOne || !sawUntagged {
		t.Errorf("VisibleItems() with ActiveLocation=1 missing expected items: sawTownOne=%v sawUntagged=%v", sawTownOne, sawUntagged)
	}
	if f.VisibleSize() != 2 {
		t.Errorf("VisibleSize() = %d, want 2", f.VisibleSize())
	}

	f.ActiveLocation = 0
	if got := f.VisibleSize(); got != 3 {
		t.Errorf("VisibleSize() with ActiveLocation=0 (no town selected) = %d, want 3 (everything visible)", got)
	}
}

func TestFreight_VisibleItemByTemplateID(t *testing.T) {
	f := NewFreight(0x10000001, freightTestTemplates())

	f.ActiveLocation = 1
	f.AddNew(freightTestItemID, 1, 0x20000001)
	f.ActiveLocation = 2

	if got := f.VisibleItemByTemplateID(freightTestItemID); got != nil {
		t.Errorf("VisibleItemByTemplateID() = %v, want nil (item belongs to a different town)", got)
	}

	f.ActiveLocation = 1
	if got := f.VisibleItemByTemplateID(freightTestItemID); got == nil {
		t.Errorf("VisibleItemByTemplateID() = nil, want the town-1 item")
	}
}

func TestFreight_ValidateCapacity_ScopedToVisibleItems(t *testing.T) {
	f := NewFreight(0x10000001, freightTestTemplates())
	f.SlotLimit = 1

	f.ActiveLocation = 1
	f.AddNew(freightTestItemID, 1, 0x20000001)
	f.ActiveLocation = 2

	// The visible (town-2) portion is empty, so there's room even though
	// the container holds an item overall.
	if !f.ValidateCapacity(1) {
		t.Errorf("ValidateCapacity(1) should pass: the town-1 item doesn't count against town-2's visible capacity")
	}

	f.AddNew(freightTestItemID, 1, 0x20000002)
	if f.ValidateCapacity(1) {
		t.Errorf("ValidateCapacity(1) should fail once the visible (town-2) portion is at SlotLimit")
	}
	if !f.ValidateCapacity(0) {
		t.Errorf("ValidateCapacity(0) should always be true regardless of the limit")
	}
}
