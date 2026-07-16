package inventory

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

type testIDs struct{ next int32 }

func (ids *testIDs) NextID() (int32, error) {
	ids.next++
	return ids.next, nil
}

func testTemplates() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: item.AdenaID, Kind: item.KindEtcItem, Stackable: true, Dropable: true, Tradable: true, Destroyable: true, Duration: -1, EtcItem: &item.EtcItemDetail{}},
		{ID: 20, Kind: item.KindEtcItem, Dropable: true, Tradable: true, Destroyable: true, Duration: -1, EtcItem: &item.EtcItemDetail{}},
		{ID: 30, Kind: item.KindWeapon, Slot: item.SlotRHand, Dropable: true, Tradable: true, Destroyable: true, Duration: -1, Weapon: &item.WeaponDetail{Type: item.WeaponSword}},
	})
}

func TestTransferItemPartialCreatesNewStackAndPersistenceActions(t *testing.T) {
	templates := testTemplates()
	source := itemcontainer.NewPlayerInventory(1, templates)
	target := itemcontainer.NewPetInventory(2, templates)
	stack := source.AddNew(item.AdenaID, 100, 500)
	source.DrainUpdates()
	target.DrainUpdates()

	res, ok, err := NewService(&testIDs{next: 900}).TransferItem(source, target, stack.ObjectID, 30)
	if err != nil {
		t.Fatalf("TransferItem error = %v", err)
	}
	if !ok {
		t.Fatal("TransferItem returned ok=false")
	}

	if stack.Count != 70 {
		t.Fatalf("source count = %d, want 70", stack.Count)
	}
	moved := target.ItemByTemplateID(item.AdenaID)
	if moved == nil || moved.Count != 30 || moved.OwnerID != 2 || moved.Location != item.LocationPet {
		t.Fatalf("target stack = %+v, want 30 adena in pet inventory", moved)
	}
	if moved.ObjectID != 901 {
		t.Fatalf("moved object id = %d, want allocated 901", moved.ObjectID)
	}
	if len(res.Persist) != 2 {
		t.Fatalf("persist actions = %+v, want source update and target save", res.Persist)
	}
	if res.Persist[0].Action != PersistUpdate || res.Persist[0].Item.ObjectID != stack.ObjectID || res.Persist[0].Item.Count != 70 {
		t.Fatalf("first persist = %+v, want source update", res.Persist[0])
	}
	if res.Persist[1].Action != PersistSave || res.Persist[1].Item.ObjectID != moved.ObjectID || res.Persist[1].Item.OwnerID != 2 {
		t.Fatalf("second persist = %+v, want target save", res.Persist[1])
	}
}

func TestTransferItemFullMoveUpdatesMovedInstance(t *testing.T) {
	templates := testTemplates()
	source := itemcontainer.NewPlayerInventory(1, templates)
	target := itemcontainer.NewPetInventory(2, templates)
	inst := source.AddNew(20, 1, 500)
	source.DrainUpdates()
	target.DrainUpdates()

	res, ok, err := NewService(nil).TransferItem(source, target, inst.ObjectID, 1)
	if err != nil {
		t.Fatalf("TransferItem error = %v", err)
	}
	if !ok {
		t.Fatal("TransferItem returned ok=false")
	}
	if source.ItemByObjectID(inst.ObjectID) != nil {
		t.Fatal("source still holds moved item")
	}
	if target.ItemByObjectID(inst.ObjectID) != inst || inst.OwnerID != 2 || inst.Location != item.LocationPet {
		t.Fatalf("target item = %+v, want original instance moved to pet", inst)
	}
	if len(res.Persist) != 1 || res.Persist[0].Action != PersistUpdate || res.Persist[0].Item.ObjectID != inst.ObjectID {
		t.Fatalf("persist actions = %+v, want moved item update", res.Persist)
	}
}

func TestDropItemPartialAllocatesDroppedStack(t *testing.T) {
	templates := testTemplates()
	inv := itemcontainer.NewPlayerInventory(1, templates)
	stack := inv.AddNew(item.AdenaID, 100, 500)
	inv.DrainUpdates()

	res, ok, err := NewService(&testIDs{next: 900}).DropItem(inv, stack.ObjectID, 40)
	if err != nil {
		t.Fatalf("DropItem error = %v", err)
	}
	if !ok {
		t.Fatal("DropItem returned ok=false")
	}
	if stack.Count != 60 {
		t.Fatalf("source count = %d, want 60", stack.Count)
	}
	if res.Dropped == nil || res.Dropped.ObjectID != 901 || res.Dropped.Count != 40 || res.Dropped.TemplateID != item.AdenaID {
		t.Fatalf("dropped item = %+v, want allocated 40 adena stack", res.Dropped)
	}
}

func TestToggleEquipItemEquipsAndUnequips(t *testing.T) {
	templates := testTemplates()
	inv := itemcontainer.NewPlayerInventory(1, templates)
	inst := inv.AddNew(30, 1, 500)
	inv.DrainUpdates()

	res, ok := NewService(nil).ToggleEquipItem(inv, inst.ObjectID)
	if !ok {
		t.Fatal("ToggleEquipItem equip returned ok=false")
	}
	if !res.EquipmentChanged {
		t.Fatal("ToggleEquipItem equip did not report equipment change")
	}
	if !inst.Equipped() {
		t.Fatalf("item location = %s/%d, want equipped", inst.Location, inst.LocationData)
	}

	res, ok = NewService(nil).ToggleEquipItem(inv, inst.ObjectID)
	if !ok {
		t.Fatal("ToggleEquipItem unequip returned ok=false")
	}
	if !res.EquipmentChanged {
		t.Fatal("ToggleEquipItem unequip did not report equipment change")
	}
	if inst.Equipped() {
		t.Fatalf("item location = %s/%d, want inventory", inst.Location, inst.LocationData)
	}
}

func TestUnequipBodySlotResolvesPaperdollSlot(t *testing.T) {
	templates := testTemplates()
	inv := itemcontainer.NewPlayerInventory(1, templates)
	inst := inv.AddNew(30, 1, 500)
	service := NewService(nil)
	if _, ok := service.ToggleEquipItem(inv, inst.ObjectID); !ok {
		t.Fatal("ToggleEquipItem equip returned ok=false")
	}
	inv.DrainUpdates()

	res, ok := service.UnequipBodySlot(inv, int32(item.SlotRHand))
	if !ok {
		t.Fatal("UnequipBodySlot returned ok=false")
	}
	if !res.EquipmentChanged {
		t.Fatal("UnequipBodySlot did not report equipment change")
	}
	if inst.Equipped() {
		t.Fatalf("item location = %s/%d, want inventory", inst.Location, inst.LocationData)
	}
}

func TestDestroyItemRejectsNonDestroyable(t *testing.T) {
	templates := item.NewTable([]*item.Template{{ID: 20, Kind: item.KindEtcItem, Destroyable: false, Duration: -1, EtcItem: &item.EtcItemDetail{}}})
	inv := itemcontainer.NewPlayerInventory(1, templates)
	inst := inv.AddNew(20, 1, 500)
	inv.DrainUpdates()

	_, ok := NewService(nil).DestroyItem(inv, inst.ObjectID, 1)

	if ok {
		t.Fatal("DestroyItem returned ok=true for non-destroyable item")
	}
	if inv.ItemByObjectID(inst.ObjectID) == nil {
		t.Fatal("non-destroyable item was removed")
	}
}

func TestCrystallizeItemDestroysSourceAndAddsCrystals(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 30, Kind: item.KindWeapon, Crystal: item.CrystalD, CrystalCount: 10, Destroyable: true, Duration: -1, Weapon: &item.WeaponDetail{Type: item.WeaponSword}},
		{ID: item.CrystalD.ItemID(), Kind: item.KindEtcItem, Stackable: true, Duration: -1, EtcItem: &item.EtcItemDetail{}},
	})
	inv := itemcontainer.NewPlayerInventory(1, templates)
	source := inv.AddNew(30, 1, 500)
	inv.DrainUpdates()

	res, failure, err := NewService(&testIDs{next: 900}).CrystallizeItem(inv, source.ObjectID, 1, 1)
	if err != nil {
		t.Fatalf("CrystallizeItem error = %v", err)
	}
	if failure != CrystallizeOK {
		t.Fatalf("CrystallizeItem failure = %v, want OK", failure)
	}
	if inv.ItemByObjectID(source.ObjectID) != nil {
		t.Fatal("source item still in inventory")
	}
	crystal := inv.ItemByTemplateID(item.CrystalD.ItemID())
	if crystal == nil || crystal.ObjectID != 901 || crystal.Count != 10 {
		t.Fatalf("crystal stack = %+v, want allocated 10 D crystals", crystal)
	}
	if res.SourceItemID != 30 || res.CrystalItemID != item.CrystalD.ItemID() || res.CrystalCount != 10 {
		t.Fatalf("result = %+v, want source 30 and 10 D crystals", res)
	}
}

func TestCrystallizeItemReportsGradeTooHighWithoutMutation(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 30, Kind: item.KindWeapon, Crystal: item.CrystalC, CrystalCount: 10, Destroyable: true, Duration: -1, Weapon: &item.WeaponDetail{Type: item.WeaponSword}},
		{ID: item.CrystalC.ItemID(), Kind: item.KindEtcItem, Stackable: true, Duration: -1, EtcItem: &item.EtcItemDetail{}},
	})
	inv := itemcontainer.NewPlayerInventory(1, templates)
	source := inv.AddNew(30, 1, 500)
	inv.DrainUpdates()

	_, failure, err := NewService(&testIDs{next: 900}).CrystallizeItem(inv, source.ObjectID, 1, 1)
	if err != nil {
		t.Fatalf("CrystallizeItem error = %v", err)
	}
	if failure != CrystallizeGradeTooHigh {
		t.Fatalf("CrystallizeItem failure = %v, want grade too high", failure)
	}
	if inv.ItemByObjectID(source.ObjectID) == nil {
		t.Fatal("source item was removed")
	}
}
