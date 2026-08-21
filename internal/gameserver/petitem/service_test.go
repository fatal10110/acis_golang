package petitem

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

type pickupTestOwner struct {
	world.Presence
	id int32
}

func (o *pickupTestOwner) ObjectID() int32 { return o.id }
func (o *pickupTestOwner) LevelValue() int { return 1 }

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

// Java's SummonAI.thinkPickUp() validates only slot capacity, never weight,
// before pet ground-item pickup (issue #1200) — a pet already over its
// weight limit must still succeed.
func TestPickupGroundItemSucceedsOverPetWeightLimit(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 9002, Kind: item.KindEtcItem, Stackable: true, Dropable: true, Tradable: true, Destroyable: true, Duration: -1, Weight: 50, EtcItem: &item.EtcItemDetail{}},
	})
	petInv := itemcontainer.NewPetInventory(2, templates)
	petInv.WeightLimit = 1
	pet := summon.NewPet(summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv})
	tmpl, ok := templates.Get(9002)
	if !ok {
		t.Fatal("template missing")
	}
	ground, err := grounditem.New(item.Instance{ObjectID: 900, TemplateID: 9002, Count: 1, ManaLeft: -1}, tmpl)
	if err != nil {
		t.Fatalf("ground item: %v", err)
	}

	res, failure := PickupGround(pet, petInv, ground)

	if failure != PickupOK {
		t.Fatalf("PickupGround failure = %v, want OK (Java has no weight gate on pet pickup)", failure)
	}
	petStack := petInv.ItemByTemplateID(9002)
	if petStack == nil || petStack.ObjectID != ground.ObjectID() {
		t.Fatalf("pet stack = %+v, want picked ground item", petStack)
	}
	if len(res.Persist) != 1 || res.Persist[0].Action != inventory.PersistSave {
		t.Fatalf("persist actions = %+v, want save picked stack", res.Persist)
	}
}

// SummonAI.thinkPickUp() (SummonAI.java:183) rejects pickup when the ground
// item is owned by someone other than the pet owner, mirroring invops.LootLocked
// on the player pickup path.
func TestPickupGroundRejectsLootLockedItem(t *testing.T) {
	templates := testTemplates()
	petInv := itemcontainer.NewPetInventory(2, templates)
	owner := &pickupTestOwner{id: 1}
	pet := summon.NewPet(summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv, Owner: owner})
	tmpl, ok := templates.Get(item.AdenaID)
	if !ok {
		t.Fatal("adena template missing")
	}
	ground, err := grounditem.New(item.Instance{ObjectID: 900, TemplateID: item.AdenaID, Count: 40, ManaLeft: -1, OwnerID: 99}, tmpl)
	if err != nil {
		t.Fatalf("ground item: %v", err)
	}

	res, failure := PickupGround(pet, petInv, ground)

	if failure != PickupLootLocked {
		t.Fatalf("PickupGround failure = %v, want PickupLootLocked", failure)
	}
	if petInv.ItemByTemplateID(item.AdenaID) != nil || len(res.Persist) != 0 {
		t.Fatalf("result = %+v, want no pickup", res)
	}
}

// Java checks capacity (SummonAI.java:177) before the owner/loot-lock check
// (SummonAI.java:183), so a pet inventory that is both full and facing a
// loot-locked item must report the capacity failure, matching
// network/pickup.go's herb-branch ordering convention.
func TestPickupGroundReportsCapacityBeforeLootLock(t *testing.T) {
	templates := testTemplates()
	petInv := itemcontainer.NewPetInventory(2, templates)
	petInv.SlotLimit = 1
	petInv.AddNew(20, 1, 600)
	owner := &pickupTestOwner{id: 1}
	pet := summon.NewPet(summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv, Owner: owner})
	tmpl, ok := templates.Get(item.AdenaID)
	if !ok {
		t.Fatal("adena template missing")
	}
	ground, err := grounditem.New(item.Instance{ObjectID: 900, TemplateID: item.AdenaID, Count: 40, ManaLeft: -1, OwnerID: 99}, tmpl)
	if err != nil {
		t.Fatalf("ground item: %v", err)
	}

	_, failure := PickupGround(pet, petInv, ground)

	if failure != PickupPetCannotCarryMore {
		t.Fatalf("PickupGround failure = %v, want PickupPetCannotCarryMore (capacity checked before loot lock)", failure)
	}
}

func TestPickupGroundAllowsPetOwnerLootedItem(t *testing.T) {
	templates := testTemplates()
	petInv := itemcontainer.NewPetInventory(2, templates)
	owner := &pickupTestOwner{id: 1}
	pet := summon.NewPet(summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv, Owner: owner})
	tmpl, ok := templates.Get(item.AdenaID)
	if !ok {
		t.Fatal("adena template missing")
	}
	ground, err := grounditem.New(item.Instance{ObjectID: 900, TemplateID: item.AdenaID, Count: 40, ManaLeft: -1, OwnerID: 1}, tmpl)
	if err != nil {
		t.Fatalf("ground item: %v", err)
	}

	res, failure := PickupGround(pet, petInv, ground)

	if failure != PickupOK {
		t.Fatalf("PickupGround failure = %v, want OK", failure)
	}
	if petInv.ItemByTemplateID(item.AdenaID) == nil || len(res.Persist) != 1 {
		t.Fatalf("result = %+v, want pickup", res)
	}
}

func TestPickupGroundHerbStaysOutOfPetInventory(t *testing.T) {
	templates := item.NewTable([]*item.Template{{
		ID:          9001,
		Kind:        item.KindEtcItem,
		Stackable:   true,
		Dropable:    true,
		Tradable:    true,
		Destroyable: true,
		Duration:    -1,
		EtcItem:     &item.EtcItemDetail{Type: item.EtcItemHerb},
	}})
	petInv := itemcontainer.NewPetInventory(2, templates)
	pet := summon.NewPet(summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv})
	tmpl, _ := templates.Get(9001)
	ground, err := grounditem.New(item.Instance{ObjectID: 900, TemplateID: 9001, Count: 1, ManaLeft: -1}, tmpl)
	if err != nil {
		t.Fatalf("ground item: %v", err)
	}

	res, failure := PickupGround(pet, petInv, ground)

	if failure != PickupOK {
		t.Fatalf("PickupGround failure = %v, want OK", failure)
	}
	if petInv.ItemByTemplateID(9001) != nil {
		t.Fatal("herb entered pet inventory")
	}
	if res.Herb == nil || res.Herb.TemplateID != 9001 || len(res.Persist) != 0 {
		t.Fatalf("result = %+v, want transient herb and no persistence", res)
	}
}

func TestPickupGroundRejectsLootLockedHerb(t *testing.T) {
	templates := item.NewTable([]*item.Template{{
		ID:          9001,
		Kind:        item.KindEtcItem,
		Stackable:   true,
		Dropable:    true,
		Tradable:    true,
		Destroyable: true,
		Duration:    -1,
		EtcItem:     &item.EtcItemDetail{Type: item.EtcItemHerb},
	}})
	petInv := itemcontainer.NewPetInventory(2, templates)
	owner := &pickupTestOwner{id: 1}
	pet := summon.NewPet(summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv, Owner: owner})
	tmpl, _ := templates.Get(9001)
	ground, err := grounditem.New(item.Instance{ObjectID: 900, TemplateID: 9001, Count: 1, ManaLeft: -1, OwnerID: 99}, tmpl)
	if err != nil {
		t.Fatalf("ground item: %v", err)
	}

	res, failure := PickupGround(pet, petInv, ground)

	if failure != PickupLootLocked {
		t.Fatalf("PickupGround failure = %v, want PickupLootLocked", failure)
	}
	if res.Herb != nil {
		t.Fatalf("result = %+v, want no herb use", res)
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

// TestGetFromPetLeavesEquipmentUntouchedOnFailedTransfer covers the ordering
// bug flagged on PR 1688: GetFromPet must not unequip until s.transfer
// reports GiveOK, matching where Java's own paperdoll-clearing side effect
// fires — inside ItemContainer.transferItem's removeItem call
// (ItemContainer.java:279/292), which only runs once the item is confirmed
// removed. count<=0 fails inventory.Service.TransferItem deterministically
// (service.go's TransferItem guard) without needing a real race.
func TestGetFromPetLeavesEquipmentUntouchedOnFailedTransfer(t *testing.T) {
	templates := testTemplates()
	playerInv := itemcontainer.NewPlayerInventory(1, templates)
	petInv := itemcontainer.NewPetInventory(2, templates)
	pet := summon.NewPet(summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv})
	weapon := petInv.AddNew(20, 1, 700)
	petInv.DrainUpdates()

	if _, failure := UseItem(pet, petInv, weapon.ObjectID, false); failure != UseOK {
		t.Fatalf("setup: equip failed")
	}
	if weapon.Location != item.LocationPetEquip || petInv.ItemAt(itemcontainer.RHand) != weapon {
		t.Fatalf("setup: weapon = %+v, want equipped in pet RHand", weapon)
	}

	_, ok, err := NewService(&testIDs{next: 900}).GetFromPet(petInv, playerInv, weapon.ObjectID, 0)
	if err != nil {
		t.Fatalf("GetFromPet error = %v", err)
	}
	if ok {
		t.Fatalf("GetFromPet ok = true, want false for count<=0")
	}
	if weapon.Location != item.LocationPetEquip || petInv.ItemAt(itemcontainer.RHand) != weapon {
		t.Fatalf("weapon = %+v after failed transfer, want still equipped in pet RHand", weapon)
	}
}

func testTemplates() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: item.AdenaID, Kind: item.KindEtcItem, Stackable: true, Dropable: true, Tradable: true, Destroyable: true, Duration: -1, EtcItem: &item.EtcItemDetail{}},
		{ID: 20, Kind: item.KindWeapon, Slot: item.SlotWolf, Dropable: true, Tradable: true, Destroyable: true, Duration: -1, Weapon: &item.WeaponDetail{Type: item.WeaponPet}},
		{ID: 30, Kind: item.KindWeapon, Slot: item.SlotRHand, Dropable: true, Tradable: true, Destroyable: true, Duration: -1, Weapon: &item.WeaponDetail{Type: item.WeaponSword}},
	})
}
