package itemcontainer

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

const (
	adenaTemplateID  int32 = item.AdenaID
	daggerTemplateID int32 = 100
	potionTemplateID int32 = 200
)

func testTemplates() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: adenaTemplateID, Name: "Adena", Kind: item.KindEtcItem, Stackable: true, Dropable: true, Tradable: true, Sellable: true, Destroyable: true, Depositable: true, EtcItem: &item.EtcItemDetail{}},
		{ID: daggerTemplateID, Name: "Dagger", Kind: item.KindWeapon, Slot: item.SlotRHand, Dropable: true, Tradable: true, Sellable: true, Destroyable: true, Depositable: true, Weapon: &item.WeaponDetail{}},
		{ID: potionTemplateID, Name: "Potion", Kind: item.KindEtcItem, Stackable: true, Dropable: true, Tradable: true, Sellable: true, Destroyable: true, Depositable: true, EtcItem: &item.EtcItemDetail{}},
	})
}

func newTestContainer() *Container {
	return NewContainer(0x10000001, item.LocationWarehouse, testTemplates())
}

func TestContainer_AddNew_MergesStackable(t *testing.T) {
	c := newTestContainer()

	first := c.AddNew(adenaTemplateID, 100, 0x20000001)
	if first == nil || first.Count != 100 {
		t.Fatalf("AddNew() = %+v, want count 100", first)
	}

	second := c.AddNew(adenaTemplateID, 50, 0x20000002)
	if second != first {
		t.Fatalf("AddNew() on a stackable template should return the pre-existing stack")
	}
	if second.Count != 150 {
		t.Errorf("Count = %d, want 150 after merge", second.Count)
	}
	if c.Size() != 1 {
		t.Errorf("Size() = %d, want 1 (merged into one stack)", c.Size())
	}
}

func TestContainer_AddNew_NonStackableStaysSeparate(t *testing.T) {
	c := newTestContainer()

	c.AddNew(daggerTemplateID, 1, 0x20000001)
	c.AddNew(daggerTemplateID, 1, 0x20000002)

	if c.Size() != 2 {
		t.Errorf("Size() = %d, want 2 (non-stackable items never merge)", c.Size())
	}
}

func TestContainer_DestroyItem(t *testing.T) {
	c := newTestContainer()
	inst := c.AddNew(adenaTemplateID, 100, 0x20000001)

	if got := c.DestroyItem(inst, 40); got != inst || inst.Count != 60 {
		t.Fatalf("partial destroy: Count = %d, want 60", inst.Count)
	}
	if got := c.DestroyItem(inst, 100); got != nil {
		t.Errorf("destroying more than held should return nil, got %+v", got)
	}
	if got := c.DestroyItem(inst, 60); got != inst {
		t.Fatalf("destroying exactly the held count should return the instance")
	}
	if inst.Count != 0 || inst.Location != item.LocationVoid || inst.OwnerID != 0 {
		t.Errorf("fully destroyed instance state = %+v, want Count=0 Location=VOID OwnerID=0", inst)
	}
	if c.Size() != 0 {
		t.Errorf("Size() = %d, want 0 after full destroy", c.Size())
	}
}

func TestContainer_ItemCount(t *testing.T) {
	c := newTestContainer()
	c.AddNew(potionTemplateID, 5, 0x20000001)
	d1 := c.AddNew(daggerTemplateID, 1, 0x20000002)
	d1.EnchantLevel = 3
	c.AddNew(daggerTemplateID, 1, 0x20000003)

	if got := c.ItemCount(potionTemplateID, -1, true); got != 5 {
		t.Errorf("stackable ItemCount() = %d, want 5", got)
	}
	if got := c.ItemCount(daggerTemplateID, -1, true); got != 2 {
		t.Errorf("non-stackable ItemCount() = %d, want 2", got)
	}
	if got := c.ItemCount(daggerTemplateID, 3, true); got != 1 {
		t.Errorf("enchant-filtered ItemCount() = %d, want 1", got)
	}
}

func TestContainer_ItemCount_ExcludesEquipped(t *testing.T) {
	c := newTestContainer()
	inst := c.AddNew(daggerTemplateID, 1, 0x20000001)
	inst.Location = item.LocationPaperdoll

	if got := c.ItemCount(daggerTemplateID, -1, false); got != 0 {
		t.Errorf("ItemCount(includeEquipped=false) = %d, want 0", got)
	}
	if got := c.ItemCount(daggerTemplateID, -1, true); got != 1 {
		t.Errorf("ItemCount(includeEquipped=true) = %d, want 1", got)
	}
}

func TestContainer_Adena(t *testing.T) {
	c := newTestContainer()
	if got := c.Adena(); got != 0 {
		t.Fatalf("Adena() on empty container = %d, want 0", got)
	}
	c.AddNew(adenaTemplateID, 12345, 0x20000001)
	if got := c.Adena(); got != 12345 {
		t.Errorf("Adena() = %d, want 12345", got)
	}
}

func TestContainer_Transfer_FullyMergesIntoExistingStack(t *testing.T) {
	src := newTestContainer()
	dst := NewContainer(0x10000002, item.LocationWarehouse, testTemplates())

	inst := src.AddNew(adenaTemplateID, 100, 0x20000001)
	dst.AddNew(adenaTemplateID, 5, 0x20000002)

	result, freedID, freed := src.Transfer(inst.ObjectID, 100, dst, 0)
	if result == nil || result.Count != 105 {
		t.Fatalf("Transfer() result = %+v, want count 105", result)
	}
	if !freed || freedID != inst.ObjectID {
		t.Errorf("fully transferring a stack that merges elsewhere should free the source object id; got freed=%v id=%d", freed, freedID)
	}
	if src.Size() != 0 {
		t.Errorf("source Size() = %d, want 0", src.Size())
	}
}

func TestContainer_Transfer_PartialCreatesNewInstance(t *testing.T) {
	src := newTestContainer()
	dst := NewContainer(0x10000002, item.LocationWarehouse, testTemplates())

	inst := src.AddNew(adenaTemplateID, 100, 0x20000001)

	result, _, freed := src.Transfer(inst.ObjectID, 30, dst, 0x30000001)
	if result == nil || result.Count != 30 || result.ObjectID != 0x30000001 {
		t.Fatalf("Transfer() result = %+v, want a new instance with count 30", result)
	}
	if freed {
		t.Errorf("a partial transfer must not free the source object id")
	}
	if inst.Count != 70 {
		t.Errorf("source Count = %d, want 70 remaining", inst.Count)
	}
}

func TestContainer_Transfer_FullyMovesNonStackableWithoutNewID(t *testing.T) {
	src := newTestContainer()
	dst := NewContainer(0x10000002, item.LocationWarehouse, testTemplates())

	inst := src.AddNew(daggerTemplateID, 1, 0x20000001)

	result, _, freed := src.Transfer(inst.ObjectID, 1, dst, 0)
	if result != inst {
		t.Fatalf("Transfer() of a whole non-stackable item should move the same instance, got %+v", result)
	}
	if freed {
		t.Errorf("moving the whole instance itself must not report a freed id")
	}
	if dst.ItemByObjectID(inst.ObjectID) != inst {
		t.Errorf("destination should now hold the transferred instance")
	}
	if src.Size() != 0 {
		t.Errorf("source Size() = %d, want 0", src.Size())
	}
}

func TestContainer_ValidateCapacity(t *testing.T) {
	c := newTestContainer()
	if !c.ValidateCapacity(1000) {
		t.Errorf("ValidateCapacity() with SlotLimit=0 (unlimited) should always be true")
	}

	c.SlotLimit = 2
	c.AddNew(daggerTemplateID, 1, 0x20000001)
	if !c.ValidateCapacity(1) {
		t.Errorf("ValidateCapacity(1) with 1/2 slots used should be true")
	}
	c.AddNew(potionTemplateID, 1, 0x20000002)
	if c.ValidateCapacity(1) {
		t.Errorf("ValidateCapacity(1) with 2/2 slots used should be false")
	}
	if !c.ValidateCapacity(0) {
		t.Errorf("ValidateCapacity(0) should always be true regardless of the limit")
	}
}
