package petitem

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

type testIDs struct{ next int32 }

func (ids *testIDs) NextID() (int32, error) {
	ids.next++
	return ids.next, nil
}

type testOwner struct{ x, y, z int }

func (o testOwner) Position() (int, int, int) { return o.x, o.y, o.z }

func TestForbiddenForPetRejectsNonDropableItem(t *testing.T) {
	tmpl := &item.Template{ID: 9000, Kind: item.KindEtcItem, Dropable: false, Destroyable: true, Tradable: true, EtcItem: &item.EtcItemDetail{}}
	inst := &item.Instance{ObjectID: 500, TemplateID: tmpl.ID, Count: 1}

	if !ForbiddenForPet(inst, tmpl) {
		t.Fatal("ForbiddenForPet returned false for non-dropable item")
	}
}

func TestGiveToPetTransfersAndReportsPersistence(t *testing.T) {
	templates := testTemplates()
	playerInv := itemcontainer.NewPlayerInventory(1, templates)
	petInv := itemcontainer.NewPetInventory(2, templates)
	pet := summon.NewPet(summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv})
	stack := playerInv.AddNew(item.AdenaID, 100, 500)
	playerInv.DrainUpdates()
	petInv.DrainUpdates()

	res, failure, err := NewService(&testIDs{next: 900}).GiveToPet(playerInv, petInv, pet, testOwner{}, stack.ObjectID, 30)
	if err != nil {
		t.Fatalf("GiveToPet error = %v", err)
	}
	if failure != GiveOK {
		t.Fatalf("GiveToPet failure = %v, want OK", failure)
	}
	petStack := petInv.ItemByTemplateID(item.AdenaID)
	if stack.Count != 70 || petStack == nil || petStack.Count != 30 || petStack.OwnerID != 2 || petStack.Location != item.LocationPet {
		t.Fatalf("transfer state source=%+v petStack=%+v, want 70 and 30 in pet", stack, petStack)
	}
	if len(res.Persist) != 2 || res.Persist[0].Action != inventory.PersistUpdate || res.Persist[1].Action != inventory.PersistSave {
		t.Fatalf("persist actions = %+v, want update and save", res.Persist)
	}
}

func TestGiveToPetChecksCapacityBeforeMutation(t *testing.T) {
	templates := testTemplates()
	playerInv := itemcontainer.NewPlayerInventory(1, templates)
	petInv := itemcontainer.NewPetInventory(2, templates)
	petInv.SlotLimit = 1
	petInv.AddNew(20, 1, 600)
	pet := summon.NewPet(summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv})
	inst := playerInv.AddNew(30, 1, 500)

	_, failure, err := NewService(nil).GiveToPet(playerInv, petInv, pet, testOwner{}, inst.ObjectID, 1)
	if err != nil {
		t.Fatalf("GiveToPet error = %v", err)
	}
	if failure != GivePetCannotCarryMore {
		t.Fatalf("GiveToPet failure = %v, want capacity failure", failure)
	}
	if playerInv.ItemByObjectID(inst.ObjectID) == nil {
		t.Fatal("item moved despite capacity failure")
	}
}

func TestGiveToPetChecksDistanceBeforeMutation(t *testing.T) {
	templates := testTemplates()
	playerInv := itemcontainer.NewPlayerInventory(1, templates)
	petInv := itemcontainer.NewPetInventory(2, templates)
	pet := summon.NewPet(summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv})
	stack := playerInv.AddNew(item.AdenaID, 100, 500)

	_, failure, err := NewService(nil).GiveToPet(playerInv, petInv, pet, testOwner{x: GiveInteractionDistance + 1}, stack.ObjectID, 30)
	if err != nil {
		t.Fatalf("GiveToPet error = %v", err)
	}
	if failure != GiveTooFar {
		t.Fatalf("GiveToPet failure = %v, want too far", failure)
	}
	if stack.Count != 100 || petInv.ItemByTemplateID(item.AdenaID) != nil {
		t.Fatalf("too-far transfer mutated source=%+v petStack=%+v", stack, petInv.ItemByTemplateID(item.AdenaID))
	}
}

func TestPickupGroundItemAddsToPetAndReportsPersistence(t *testing.T) {
	templates := testTemplates()
	petInv := itemcontainer.NewPetInventory(2, templates)
	pet := summon.NewPet(summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv})
	tmpl, ok := templates.Get(item.AdenaID)
	if !ok {
		t.Fatal("adena template missing")
	}
	ground, err := grounditem.New(item.Instance{ObjectID: 900, TemplateID: item.AdenaID, Count: 40, ManaLeft: -1}, tmpl)
	if err != nil {
		t.Fatalf("ground item: %v", err)
	}

	res, failure := PickupGround(pet, petInv, ground)

	if failure != PickupOK {
		t.Fatalf("PickupGround failure = %v, want OK", failure)
	}
	petStack := petInv.ItemByTemplateID(item.AdenaID)
	if petStack == nil || petStack.ObjectID != ground.ObjectID() || petStack.Count != 40 || petStack.OwnerID != 2 || petStack.Location != item.LocationPet {
		t.Fatalf("pet stack = %+v, want picked ground item", petStack)
	}
	if len(res.Persist) != 1 || res.Persist[0].Action != inventory.PersistSave || res.Persist[0].Item != petStack {
		t.Fatalf("persist actions = %+v, want save picked stack", res.Persist)
	}
}

func TestUseItemEquipsAndUnequipsPetWeapon(t *testing.T) {
	templates := testTemplates()
	petInv := itemcontainer.NewPetInventory(2, templates)
	pet := summon.NewPet(summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv})
	weapon := petInv.AddNew(20, 1, 700)
	petInv.DrainUpdates()

	res, failure := UseItem(pet, petInv, weapon.ObjectID, false)
	if failure != UseOK {
		t.Fatalf("UseItem failure = %v, want OK", failure)
	}
	if res.Outcome != Equipped || weapon.Location != item.LocationPetEquip || weapon.LocationData != itemcontainer.RHand {
		t.Fatalf("equip result=%+v weapon=%+v, want equipped RHand", res, weapon)
	}
	if len(res.Persist) != 1 || res.Persist[0].Action != inventory.PersistUpdate || res.Persist[0].Item != weapon {
		t.Fatalf("persist actions = %+v, want weapon update", res.Persist)
	}

	res, failure = UseItem(pet, petInv, weapon.ObjectID, false)
	if failure != UseOK {
		t.Fatalf("second UseItem failure = %v, want OK", failure)
	}
	if res.Outcome != Unequipped || weapon.Location != item.LocationPet {
		t.Fatalf("unequip result=%+v weapon=%+v, want pet inventory", res, weapon)
	}
}

func testTemplates() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: item.AdenaID, Kind: item.KindEtcItem, Stackable: true, Dropable: true, Tradable: true, Destroyable: true, Duration: -1, EtcItem: &item.EtcItemDetail{}},
		{ID: 20, Kind: item.KindWeapon, Slot: item.SlotWolf, Dropable: true, Tradable: true, Destroyable: true, Duration: -1, Weapon: &item.WeaponDetail{Type: item.WeaponPet}},
		{ID: 30, Kind: item.KindWeapon, Slot: item.SlotRHand, Dropable: true, Tradable: true, Destroyable: true, Duration: -1, Weapon: &item.WeaponDetail{Type: item.WeaponSword}},
	})
}
