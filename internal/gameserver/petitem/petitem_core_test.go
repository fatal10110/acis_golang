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

// ---- from service_test.go ----
type pickupTestOwner struct {
	world.Presence
	id int32
}

func (o *pickupTestOwner) ObjectID() int32 { return o.id }

func mustTestPet(t *testing.T, cfg summon.PetConfig) *summon.Actor {
	t.Helper()
	pet, err := summon.NewPet(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return pet
}
func (o *pickupTestOwner) LevelValue() int { return 1 }
func (o *pickupTestOwner) InCombat() bool  { return false }

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
	pet := mustTestPet(t, summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv})
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
	pet := mustTestPet(t, summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv})
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
	pet := mustTestPet(t, summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv})
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
	pet := mustTestPet(t, summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv})
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
	pet := mustTestPet(t, summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv})
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
	pet := mustTestPet(t, summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv, Owner: owner})
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
	pet := mustTestPet(t, summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv, Owner: owner})
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
	pet := mustTestPet(t, summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv, Owner: owner})
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
	pet := mustTestPet(t, summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv})
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
	pet := mustTestPet(t, summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv, Owner: owner})
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
	pet := mustTestPet(t, summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv})
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
	pet := mustTestPet(t, summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv})
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

// TestUseItemRejectsConditionFailureOnUnequippedItem covers
// RequestPetUseItem.java:40's item.getItem().checkCondition(pet, pet, true)
// gate: an unequipped pet weapon whose template carries a <cond> the pet
// fails must not equip, matching Item.checkCondition's PET_CANNOT_USE_ITEM
// side effect on a Summon effector (Item.java:455-459).
func TestUseItemRejectsConditionFailureOnUnequippedItem(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 21, Kind: item.KindWeapon, Slot: item.SlotWolf, Dropable: true, Tradable: true, Destroyable: true, Duration: -1,
			Weapon:        &item.WeaponDetail{Type: item.WeaponPet},
			UseConditions: []item.UseCondition{{Root: item.Condition{Kind: "player", Attrs: map[string]string{"level": "50"}}}}},
	})
	petInv := itemcontainer.NewPetInventory(2, templates)
	pet := mustTestPet(t, summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv, Level: 10})
	weapon := petInv.AddNew(21, 1, 700)
	petInv.DrainUpdates()

	res, failure := UseItem(pet, petInv, weapon.ObjectID, false)
	if failure != UsePetCannotUseItem || res.ItemID != 21 {
		t.Fatalf("UseItem = (%+v, %v), want UsePetCannotUseItem for level-gated weapon on an under-level pet", res, failure)
	}
	if weapon.Location == item.LocationPetEquip {
		t.Fatalf("weapon = %+v, want left unequipped after condition failure", weapon)
	}
}

// TestUseItemAllowsConditionSuccessOnUnequippedItem covers the passing side
// of the same gate: a pet meeting the level condition still equips normally.
func TestUseItemAllowsConditionSuccessOnUnequippedItem(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 21, Kind: item.KindWeapon, Slot: item.SlotWolf, Dropable: true, Tradable: true, Destroyable: true, Duration: -1,
			Weapon:        &item.WeaponDetail{Type: item.WeaponPet},
			UseConditions: []item.UseCondition{{Root: item.Condition{Kind: "player", Attrs: map[string]string{"level": "50"}}}}},
	})
	petInv := itemcontainer.NewPetInventory(2, templates)
	pet := mustTestPet(t, summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv, Level: 50})
	weapon := petInv.AddNew(21, 1, 700)
	petInv.DrainUpdates()

	res, failure := UseItem(pet, petInv, weapon.ObjectID, false)
	if failure != UseOK || res.Outcome != Equipped {
		t.Fatalf("UseItem = (%+v, %v), want equipped for a pet meeting the level condition", res, failure)
	}
}

// TestUseItemConditionGateSkipsAlreadyEquippedItem covers the reference's
// !item.isEquipped() guard: unequipping a currently worn item never runs
// checkCondition, so an already-equipped item that would now fail its own
// condition (e.g. the pet fell below the level requirement) still unequips.
func TestUseItemConditionGateSkipsAlreadyEquippedItem(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 21, Kind: item.KindWeapon, Slot: item.SlotWolf, Dropable: true, Tradable: true, Destroyable: true, Duration: -1,
			Weapon:        &item.WeaponDetail{Type: item.WeaponPet},
			UseConditions: []item.UseCondition{{Root: item.Condition{Kind: "player", Attrs: map[string]string{"level": "50"}}}}},
	})
	petInv := itemcontainer.NewPetInventory(2, templates)
	// The pet is under-level from the start (unlike the equip case, level
	// never needs to change): the reference's !item.isEquipped() guard skips
	// checkCondition entirely for the unequip path, so it must never run.
	pet := mustTestPet(t, summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv, Level: 10})
	weapon := petInv.AddNew(21, 1, 700)
	tmpl, _ := templates.Get(21)
	petInv.SetPaperdollItem(itemcontainer.RHand, weapon, tmpl)
	petInv.DrainUpdates()

	res, failure := UseItem(pet, petInv, weapon.ObjectID, false)
	if failure != UseOK || res.Outcome != Unequipped {
		t.Fatalf("UseItem = (%+v, %v), want unequipped without re-checking the condition", res, failure)
	}
}

// TestUseItemConditionGateAppliesToConsumableDispatch covers the reference's
// gate applying to the non-equipment (etc-item) branch too: a food/potion
// template's failed <cond> must reject before UseConsumable dispatch.
func TestUseItemConditionGateAppliesToConsumableDispatch(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1061, Kind: item.KindEtcItem, Stackable: true, Dropable: true, Tradable: true, Destroyable: true, Duration: -1,
			EtcItem:       &item.EtcItemDetail{Type: item.EtcItemPotion, Handler: "ItemSkills"},
			UseConditions: []item.UseCondition{{Root: item.Condition{Kind: "player", Attrs: map[string]string{"level": "50"}}}}},
	})
	petInv := itemcontainer.NewPetInventory(2, templates)
	pet := mustTestPet(t, summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv, Level: 10})
	potion := petInv.AddNew(1061, 1, 700)
	petInv.DrainUpdates()

	res, failure := UseItem(pet, petInv, potion.ObjectID, false)
	if failure != UsePetCannotUseItem || res.ItemID != 1061 {
		t.Fatalf("UseItem = (%+v, %v), want UsePetCannotUseItem for a level-gated potion on an under-level pet", res, failure)
	}
}

func testTemplates() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: item.AdenaID, Kind: item.KindEtcItem, Stackable: true, Dropable: true, Tradable: true, Destroyable: true, Duration: -1, EtcItem: &item.EtcItemDetail{}},
		{ID: 20, Kind: item.KindWeapon, Slot: item.SlotWolf, Dropable: true, Tradable: true, Destroyable: true, Duration: -1, Weapon: &item.WeaponDetail{Type: item.WeaponPet}},
		{ID: 30, Kind: item.KindWeapon, Slot: item.SlotRHand, Dropable: true, Tradable: true, Destroyable: true, Duration: -1, Weapon: &item.WeaponDetail{Type: item.WeaponSword}},
		{ID: 1060, Kind: item.KindEtcItem, Stackable: true, Dropable: true, Tradable: true, Destroyable: true, Duration: -1, EtcItem: &item.EtcItemDetail{Type: item.EtcItemPotion, Handler: "ItemSkills"}},
		{ID: 2515, Kind: item.KindEtcItem, Stackable: true, Dropable: true, Tradable: true, Destroyable: true, Duration: -1, EtcItem: &item.EtcItemDetail{Handler: "PetFoods"}},
		{ID: 4038, Kind: item.KindEtcItem, Stackable: true, Dropable: true, Tradable: true, Destroyable: true, Duration: -1, EtcItem: &item.EtcItemDetail{Handler: "PetFoods"}},
	})
}

// TestUseItemDispatchesEligibleConsumable pins #1582: a non-equipment item
// this pet can eat (matches its template food ids) or that is a potion must
// report UseConsumable for the network layer to dispatch to its etc-item
// handler, not fall through to UsePetCannotUseItem.
func TestUseItemDispatchesEligibleConsumable(t *testing.T) {
	templates := testTemplates()
	petInv := itemcontainer.NewPetInventory(2, templates)
	pet := mustTestPet(t, summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv, Food1: 2515})

	potion := petInv.AddNew(1060, 1, 700)
	petInv.DrainUpdates()
	res, failure := UseItem(pet, petInv, potion.ObjectID, false)
	if failure != UseConsumable || res.ItemID != 1060 {
		t.Fatalf("potion UseItem = (%+v, %v), want UseConsumable for template 1060", res, failure)
	}

	food := petInv.AddNew(2515, 1, 701)
	petInv.DrainUpdates()
	res, failure = UseItem(pet, petInv, food.ObjectID, false)
	if failure != UseConsumable || res.ItemID != 2515 {
		t.Fatalf("food UseItem = (%+v, %v), want UseConsumable for template 2515 (matches pet's Food1)", res, failure)
	}
}

// TestUseItemRejectsIneligibleFood covers PetTemplate.canEatFood's negative
// case: a PetFoods item whose id doesn't match this pet's configured food
// ids is not eligible, matching RequestPetUseItem.java's PET_CANNOT_USE_ITEM
// gate rather than dispatching it.
func TestUseItemRejectsIneligibleFood(t *testing.T) {
	templates := testTemplates()
	petInv := itemcontainer.NewPetInventory(2, templates)
	pet := mustTestPet(t, summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv, Food1: 2515})

	food := petInv.AddNew(4038, 1, 700)
	petInv.DrainUpdates()

	res, failure := UseItem(pet, petInv, food.ObjectID, false)
	if failure != UsePetCannotUseItem || res.ItemID != 0 {
		t.Fatalf("UseItem = (%+v, %v), want UsePetCannotUseItem for food id not matching pet's Food1/Food2", res, failure)
	}
}
