package itemcontainer

import (
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// Template ids used across the equip tests below.
const (
	swordID      int32 = 1  // one-handed, SlotRHand
	twoHandID    int32 = 2  // two-handed, SlotLRHand
	shieldID     int32 = 3  // SlotLHand, armor shield
	bowID        int32 = 4  // SlotLRHand, weapon bow
	arrowID      int32 = 5  // SlotNone-equivalent etc item, arrow
	rodID        int32 = 6  // SlotLRHand, fishing rod
	lureID       int32 = 7  // etc item, lure
	earringID    int32 = 8  // SlotLREar
	ringID       int32 = 9  // SlotLRFinger
	chestLightID int32 = 10 // SlotChest, light armor
	chestHeavyID int32 = 11 // SlotChest, heavy armor
	legsLightID  int32 = 12 // SlotLegs, light armor
	fullArmorID  int32 = 13 // SlotFullArmor
	allDressID   int32 = 14 // SlotAllDress
	hairAllID    int32 = 15 // SlotHairAll
	faceID       int32 = 16 // SlotFace
	hairID       int32 = 17 // SlotHair
)

func equipTestTemplates() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: swordID, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{Type: item.WeaponSword}},
		{ID: twoHandID, Kind: item.KindWeapon, Slot: item.SlotLRHand, Weapon: &item.WeaponDetail{Type: item.WeaponBigSword}},
		{ID: shieldID, Kind: item.KindArmor, Slot: item.SlotLHand, Armor: &item.ArmorDetail{Type: item.ArmorShield}},
		{ID: bowID, Kind: item.KindWeapon, Slot: item.SlotLRHand, Weapon: &item.WeaponDetail{Type: item.WeaponBow}},
		{ID: arrowID, Kind: item.KindEtcItem, Slot: item.SlotLHand, EtcItem: &item.EtcItemDetail{Type: item.EtcItemArrow}},
		{ID: rodID, Kind: item.KindWeapon, Slot: item.SlotLRHand, Weapon: &item.WeaponDetail{Type: item.WeaponFishingRod}},
		{ID: lureID, Kind: item.KindEtcItem, Slot: item.SlotLHand, EtcItem: &item.EtcItemDetail{Type: item.EtcItemLure}},
		{ID: earringID, Kind: item.KindArmor, Slot: item.SlotLREar, Armor: &item.ArmorDetail{Type: item.ArmorLight}},
		{ID: ringID, Kind: item.KindArmor, Slot: item.SlotLRFinger, Armor: &item.ArmorDetail{Type: item.ArmorLight}},
		{ID: chestLightID, Kind: item.KindArmor, Slot: item.SlotChest, Armor: &item.ArmorDetail{Type: item.ArmorLight}},
		{ID: chestHeavyID, Kind: item.KindArmor, Slot: item.SlotChest, Armor: &item.ArmorDetail{Type: item.ArmorHeavy}},
		{ID: legsLightID, Kind: item.KindArmor, Slot: item.SlotLegs, Armor: &item.ArmorDetail{Type: item.ArmorLight}},
		{ID: fullArmorID, Kind: item.KindArmor, Slot: item.SlotFullArmor, Armor: &item.ArmorDetail{Type: item.ArmorHeavy}},
		{ID: allDressID, Kind: item.KindArmor, Slot: item.SlotAllDress, Armor: &item.ArmorDetail{Type: item.ArmorHeavy}},
		{ID: hairAllID, Kind: item.KindArmor, Slot: item.SlotHairAll, Armor: &item.ArmorDetail{}},
		{ID: faceID, Kind: item.KindArmor, Slot: item.SlotFace, Armor: &item.ArmorDetail{}},
		{ID: hairID, Kind: item.KindArmor, Slot: item.SlotHair, Armor: &item.ArmorDetail{}},
	})
}

// equipFixture bundles an inventory with its template table and a helper to
// equip by template id, allocating sequential object ids.
type equipFixture struct {
	inv       *Inventory
	templates *item.Table
	nextID    int32
}

func newEquipFixture() *equipFixture {
	templates := equipTestTemplates()
	return &equipFixture{
		inv:       NewPlayerInventory(0x10000001, templates),
		templates: templates,
		nextID:    0x20000001,
	}
}

func (f *equipFixture) equip(templateID int32) (*item.Instance, []*item.Instance) {
	tmpl, ok := f.templates.Get(templateID)
	if !ok {
		panic("unknown template id")
	}
	inst := f.inv.AddNew(templateID, 1, f.nextID)
	f.nextID++
	altered := f.inv.EquipItem(inst, tmpl)
	return inst, altered
}

// TestInventory_EquipItem_SlotRules walks the paperdoll slot bookkeeping
// table: paired weapons keep their offhand slot, unpaired equips clear
// conflicting slots, paired accessory slots fill left then right before
// replacing left, and whole-body pieces clear every slot they subsume.
func TestInventory_EquipItem_SlotRules(t *testing.T) {
	t.Run("two-handed clears offhand", func(t *testing.T) {
		f := newEquipFixture()
		f.equip(shieldID)
		if f.inv.ItemAt(LHand) == nil {
			t.Fatalf("shield should occupy LHand")
		}
		twoHand, altered := f.equip(twoHandID)
		if f.inv.ItemAt(RHand) != twoHand {
			t.Errorf("two-handed weapon should occupy RHand")
		}
		if f.inv.ItemAt(LHand) != nil {
			t.Errorf("equipping a two-handed weapon should clear LHand")
		}
		if len(altered) != 2 {
			t.Errorf("altered = %v, want shield unequipped + weapon equipped (2 entries)", altered)
		}
	})

	t.Run("one-handed clears existing two-handed", func(t *testing.T) {
		f := newEquipFixture()
		f.equip(twoHandID)
		sword, _ := f.equip(swordID)
		if f.inv.ItemAt(RHand) != sword {
			t.Errorf("one-handed sword should now occupy RHand")
		}
	})

	t.Run("bow arrow pairing keeps offhand", func(t *testing.T) {
		f := newEquipFixture()
		f.equip(bowID)
		arrow, _ := f.equip(arrowID)
		if f.inv.ItemAt(LHand) != arrow {
			t.Fatalf("arrow should occupy LHand")
		}
		if f.inv.ItemAt(RHand) == nil {
			t.Errorf("equipping an arrow while a bow is worn must not clear the bow")
		}
	})

	t.Run("fishing rod lure pairing keeps offhand", func(t *testing.T) {
		f := newEquipFixture()
		f.equip(rodID)
		f.equip(lureID)
		if f.inv.ItemAt(RHand) == nil {
			t.Errorf("equipping a lure while a fishing rod is worn must not clear the rod")
		}
	})

	t.Run("unpaired offhand clears two-handed", func(t *testing.T) {
		f := newEquipFixture()
		f.equip(twoHandID)
		f.equip(shieldID)
		if f.inv.ItemAt(RHand) != nil {
			t.Errorf("equipping a shield (unpaired LHand item) while two-handed should clear RHand")
		}
	})

	t.Run("ears fill first empty then replace left", func(t *testing.T) {
		f := newEquipFixture()
		first, _ := f.equip(earringID)
		if f.inv.ItemAt(LEar) != first {
			t.Fatalf("first earring should fill LEar")
		}
		second, _ := f.equip(earringID)
		if f.inv.ItemAt(REar) != second {
			t.Fatalf("second earring should fill REar")
		}
		// Both slots full: a third of the *same* template id replaces LEar
		// (matches the reference's "same id as REar -> replace LEar" rule).
		third, _ := f.equip(earringID)
		if f.inv.ItemAt(LEar) != third {
			t.Errorf("third earring of the same template should replace LEar")
		}
	})

	t.Run("fingers same shape", func(t *testing.T) {
		f := newEquipFixture()
		first, _ := f.equip(ringID)
		if f.inv.ItemAt(LFinger) != first {
			t.Fatalf("first ring should fill LFinger")
		}
		second, _ := f.equip(ringID)
		if f.inv.ItemAt(RFinger) != second {
			t.Fatalf("second ring should fill RFinger")
		}
	})

	t.Run("full armor clears legs", func(t *testing.T) {
		f := newEquipFixture()
		f.equip(legsLightID)
		full, _ := f.equip(fullArmorID)
		if f.inv.ItemAt(Chest) != full {
			t.Fatalf("full armor should occupy Chest")
		}
		if f.inv.ItemAt(Legs) != nil {
			t.Errorf("equipping full armor should clear Legs")
		}
	})

	t.Run("legs clears full armor", func(t *testing.T) {
		f := newEquipFixture()
		f.equip(fullArmorID)
		legs, _ := f.equip(legsLightID)
		if f.inv.ItemAt(Legs) != legs {
			t.Fatalf("legs should occupy Legs")
		}
		if f.inv.ItemAt(Chest) != nil {
			t.Errorf("equipping legs while full armor is worn should clear Chest")
		}
	})

	t.Run("all dress clears six slots", func(t *testing.T) {
		f := newEquipFixture()
		f.equip(legsLightID)
		f.equip(shieldID)
		f.equip(swordID)
		dress, _ := f.equip(allDressID)
		if f.inv.ItemAt(Chest) != dress {
			t.Fatalf("all-dress should occupy Chest")
		}
		for _, slot := range []int{Legs, LHand, RHand, Head, Feet, Gloves} {
			if f.inv.ItemAt(slot) != nil {
				t.Errorf("all-dress should clear paperdoll slot %d", slot)
			}
		}
	})

	t.Run("hairall clears face and vice versa", func(t *testing.T) {
		f := newEquipFixture()
		f.equip(faceID)
		hairAll, _ := f.equip(hairAllID)
		if f.inv.ItemAt(Hair) != hairAll {
			t.Fatalf("hairall should occupy Hair")
		}
		if f.inv.ItemAt(Face) != nil {
			t.Errorf("equipping hairall should clear Face")
		}
		hair, _ := f.equip(hairID)
		if f.inv.ItemAt(Hair) != hair {
			t.Fatalf("hair should occupy Hair")
		}
	})
}

func TestInventory_PackageSendableItems(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: item.AdenaID, Kind: item.KindEtcItem, Stackable: true, Tradable: true, EtcItem: &item.EtcItemDetail{}},
		{ID: potionTemplateID, Kind: item.KindEtcItem, Stackable: true, Tradable: true, EtcItem: &item.EtcItemDetail{}},
		{ID: 300, Kind: item.KindEtcItem, Stackable: true, Tradable: true, EtcItem: &item.EtcItemDetail{Type: item.EtcItemQuest}},
		{ID: 400, Kind: item.KindEtcItem, Stackable: true, Tradable: false, EtcItem: &item.EtcItemDetail{}},
		{ID: 600, Kind: item.KindEtcItem, Tradable: true, EtcItem: &item.EtcItemDetail{}},
		{ID: 601, Kind: item.KindEtcItem, Tradable: true, EtcItem: &item.EtcItemDetail{}},
	})
	inv := NewPlayerInventory(1, templates)
	inv.AddNew(item.AdenaID, 100, 500)
	inv.AddNew(potionTemplateID, 3, 501)
	equipped := inv.AddNew(600, 1, 502)
	warehouse := inv.AddNew(601, 1, 503)
	inv.AddNew(300, 1, 504)
	inv.AddNew(400, 1, 505)
	missing := &item.Instance{ObjectID: 506, TemplateID: 999, Count: 1}
	inv.Add(missing)

	equipped.Location = item.LocationPaperdoll
	warehouse.Location = item.LocationWarehouse

	items := inv.PackageSendableItems()
	if len(items) != 2 {
		t.Fatalf("PackageSendableItems() returned %d items, want 2", len(items))
	}
	if items[0].ObjectID != 500 || items[1].ObjectID != 501 {
		t.Fatalf("PackageSendableItems() object ids = %d,%d; want 500,501", items[0].ObjectID, items[1].ObjectID)
	}
}

func TestInventory_DropItem_PartialSplitsIntoNewInstance(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
	})
	inv := NewPlayerInventory(0x10000001, templates)
	inst := inv.AddNew(1, 100, 0x20000001)

	dropped := inv.DropItem(inst.ObjectID, 30, 0x30000001)
	if dropped == nil || dropped.ObjectID != 0x30000001 || dropped.Count != 30 {
		t.Fatalf("DropItem() = %+v, want a new instance carrying 30 units", dropped)
	}
	if inst.Count != 70 {
		t.Errorf("remaining stack Count = %d, want 70", inst.Count)
	}
	if inv.ItemByObjectID(inst.ObjectID) != inst {
		t.Errorf("the original stack should stay in the inventory")
	}
}

func TestInventory_DropItem_FullyRemovesInstance(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{}},
	})
	inv := NewPlayerInventory(0x10000001, templates)
	inst := inv.AddNew(1, 1, 0x20000001)
	tmpl, _ := templates.Get(1)
	inv.EquipItem(inst, tmpl)

	dropped := inv.DropItem(inst.ObjectID, 1, 0)
	if dropped != inst {
		t.Fatalf("DropItem() = %+v, want the original instance back", dropped)
	}
	if inst.OwnerID != 0 || inst.Location != item.LocationVoid {
		t.Errorf("dropped instance state = %+v, want OwnerID=0 Location=VOID", inst)
	}
	if inv.ItemAt(RHand) != nil {
		t.Errorf("dropping an equipped item should unequip it first")
	}
	if inv.Size() != 0 {
		t.Errorf("Size() = %d, want 0", inv.Size())
	}
}

func TestInventory_TransferItemPartialQueuesSourceAndTargetUpdates(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
	})
	playerInv := NewPlayerInventory(0x10000001, templates)
	petInv := NewPetInventory(0x20000001, templates)
	source := playerInv.AddNew(1, 100, 0x30000001)
	playerInv.DrainUpdates()

	result, freedID, freed := playerInv.TransferItem(source.ObjectID, 30, petInv, 0x40000001)
	if result == nil || result.ObjectID != 0x40000001 || result.Count != 30 {
		t.Fatalf("TransferItem() result = %+v, want new pet stack with 30 units", result)
	}
	if freed || freedID != 0 {
		t.Fatalf("TransferItem() freed = (%d, %v), want none for partial transfer", freedID, freed)
	}
	if source.Count != 70 {
		t.Fatalf("source Count = %d, want 70", source.Count)
	}

	sourceUpdates := playerInv.DrainUpdates()
	if len(sourceUpdates) != 1 || sourceUpdates[0].State != UpdateModified || sourceUpdates[0].ObjectID != source.ObjectID || sourceUpdates[0].Count != 70 {
		t.Fatalf("source updates = %+v, want one modified update for remaining source stack", sourceUpdates)
	}
	targetUpdates := petInv.DrainUpdates()
	if len(targetUpdates) != 1 || targetUpdates[0].State != UpdateAdded || targetUpdates[0].ObjectID != result.ObjectID || targetUpdates[0].Count != 30 {
		t.Fatalf("target updates = %+v, want one added update for pet stack", targetUpdates)
	}
}

func TestInventory_TransferItemFullIntoExistingStackQueuesRemoveAndModify(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
	})
	playerInv := NewPlayerInventory(0x10000001, templates)
	petInv := NewPetInventory(0x20000001, templates)
	source := playerInv.AddNew(1, 20, 0x30000001)
	existing := petInv.AddNew(1, 5, 0x40000001)
	playerInv.DrainUpdates()
	petInv.DrainUpdates()

	result, freedID, freed := playerInv.TransferItem(source.ObjectID, 20, petInv, 0)
	if result != existing {
		t.Fatalf("TransferItem() result = %+v, want existing pet stack", result)
	}
	if !freed || freedID != source.ObjectID {
		t.Fatalf("TransferItem() freed = (%d, %v), want source object freed", freedID, freed)
	}
	if existing.Count != 25 {
		t.Fatalf("existing pet stack Count = %d, want 25", existing.Count)
	}
	if playerInv.ItemByObjectID(source.ObjectID) != nil {
		t.Fatalf("source stack should leave player inventory")
	}

	sourceUpdates := playerInv.DrainUpdates()
	if len(sourceUpdates) != 1 || sourceUpdates[0].State != UpdateRemoved || sourceUpdates[0].ObjectID != source.ObjectID || sourceUpdates[0].Count != 20 {
		t.Fatalf("source updates = %+v, want one removed update for original source stack", sourceUpdates)
	}
	targetUpdates := petInv.DrainUpdates()
	if len(targetUpdates) != 1 || targetUpdates[0].State != UpdateModified || targetUpdates[0].ObjectID != existing.ObjectID || targetUpdates[0].Count != 25 {
		t.Fatalf("target updates = %+v, want one modified update for merged pet stack", targetUpdates)
	}
}

func TestInventory_UnequipSlot(t *testing.T) {
	f := newEquipFixture()
	sword, _ := f.equip(swordID)

	old := f.inv.UnequipSlot(RHand)
	if old != sword {
		t.Fatalf("UnequipSlot() = %v, want the equipped sword", old)
	}
	if f.inv.ItemAt(RHand) != nil {
		t.Errorf("RHand should be empty after unequip")
	}
	if sword.Location != f.inv.Location() {
		t.Errorf("unequipped item should move to the inventory's base location, got %v", sword.Location)
	}
}

func TestInventory_WornMask_TwoPieceArmorRequiresMatchingType(t *testing.T) {
	f := newEquipFixture()
	chestTmpl, _ := f.templates.Get(chestLightID)

	f.equip(chestLightID)
	f.equip(legsLightID)
	if !f.inv.IsWearingType(chestTmpl.Mask()) {
		t.Errorf("matching light chest+legs should register the light-armor worn mask")
	}

	f2 := newEquipFixture()
	f2.equip(chestHeavyID)
	f2.equip(legsLightID)
	heavyTmpl, _ := f2.templates.Get(chestHeavyID)
	lightTmpl, _ := f2.templates.Get(legsLightID)
	if f2.inv.IsWearingType(heavyTmpl.Mask()) || f2.inv.IsWearingType(lightTmpl.Mask()) {
		t.Errorf("mismatched chest/legs armor types should not register either worn-type bit")
	}
}

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

func TestInventory_FindArrowForBow(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1341, Kind: item.KindEtcItem, EtcItem: &item.EtcItemDetail{Type: item.EtcItemArrow}},
	})
	inv := NewPlayerInventory(0x10000001, templates)
	inv.AddNew(1341, 40, 0x20000001)

	if got := inv.FindArrowForBow(item.CrystalD); got == nil || got.TemplateID != 1341 {
		t.Errorf("FindArrowForBow(CrystalD) = %v, want bone arrow instance", got)
	}
	if got := inv.FindArrowForBow(item.CrystalS); got != nil {
		t.Errorf("FindArrowForBow(CrystalS) = %v, want nil (no shining arrows held)", got)
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
func TestInventory_BuildAndDrainUpdates_KeepsQueueOnBuildError(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
	})
	inv := NewPetInventory(1, templates)
	inv.AddNew(1, 1, 1)
	if !inv.HasUpdates() {
		t.Fatal("expected AddNew to queue an update")
	}

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

// TestInventory_UpdateNotifierFiresOnQueuedUpdate pins the hook every queued
// inventory change relies on, matching the reference's Inventory.addUpdate
// registering with InventoryUpdateTaskManager unconditionally: the batching
// task, not the mutation's caller, decides whether and when to drain it.
func TestInventory_UpdateNotifierFiresOnQueuedUpdate(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
	})
	inv := NewPlayerInventory(0x10000001, templates)

	notified := 0
	inv.SetUpdateNotifier(func() { notified++ })

	inv.AddNew(1, 5, 0x30000001)
	if notified != 1 {
		t.Fatalf("notifier calls after AddNew = %d, want 1", notified)
	}

	// A coalesced update still has to register the inventory: the batch it
	// merges into may already have been drained.
	inv.AddNew(1, 5, 0x30000002)
	if notified != 2 {
		t.Errorf("notifier calls after a coalesced add = %d, want 2", notified)
	}

	inv.SetUpdateNotifier(nil)
	inv.AddNew(1, 5, 0x30000003)
	if notified != 2 {
		t.Errorf("notifier calls after detach = %d, want 2", notified)
	}
}

// TestInventory_UpdateNotifierSkipsNoOpMutation matches the reference's
// addUpdate, which returns before registering with the manager whenever it
// appends nothing. A mutation that queues no update — an empty paperdoll
// slot here — must not fire the notifier either, or it parks the inventory
// in the batching task with nothing to send.
func TestInventory_UpdateNotifierSkipsNoOpMutation(t *testing.T) {
	templates := item.NewTable(nil)
	inv := NewPlayerInventory(0x10000001, templates)

	notified := 0
	inv.SetUpdateNotifier(func() { notified++ })

	inv.UnequipSlot(RHand)
	if notified != 0 {
		t.Fatalf("notifier calls after unequipping an empty slot = %d, want 0", notified)
	}
}

// ---- from container_bench_test.go ----
func newFullFreight(size int) *Freight {
	f := NewFreight(0x10000001, testTemplates())
	f.ActiveLocation = 1
	for i := 0; i < size; i++ {
		inst := f.AddNew(daggerTemplateID, 1, int32(0x20000000+i))
		if i%2 == 0 {
			inst.LocationData = 2
		}
	}
	return f
}

func newWeightedInventory(size int) *Inventory {
	templates := item.NewTable([]*item.Template{
		{ID: daggerTemplateID, Kind: item.KindWeapon, Slot: item.SlotRHand, Weight: 3, Weapon: &item.WeaponDetail{}},
	})
	inv := NewPlayerInventory(0x10000001, templates)
	for i := 0; i < size; i++ {
		inv.AddNew(daggerTemplateID, 1, int32(0x20000000+i))
	}
	return inv
}

func TestFreight_VisibleItemsAllocatesOnlyResultSlice(t *testing.T) {
	f := newFullFreight(64)

	allocs := testing.AllocsPerRun(100, func() {
		_ = f.VisibleItems()
	})
	if allocs > 1 {
		t.Fatalf("VisibleItems() allocs/run = %.0f, want at most 1 result-slice allocation", allocs)
	}
}

func TestInventory_UpdateWeightDoesNotAllocateForIteration(t *testing.T) {
	inv := newWeightedInventory(64)

	allocs := testing.AllocsPerRun(100, func() {
		_ = inv.UpdateWeight()
	})
	if allocs != 0 {
		t.Fatalf("UpdateWeight() allocs/run = %.0f, want 0", allocs)
	}
}

func BenchmarkFreightValidateCapacity(b *testing.B) {
	f := newFullFreight(128)
	f.SlotLimit = 256

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = f.ValidateCapacity(1)
	}
}

func BenchmarkInventoryUpdateWeight(b *testing.B) {
	inv := newWeightedInventory(128)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = inv.UpdateWeight()
	}
}

// ---- from container_test.go ----
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

func TestContainer_AddRejectsZeroObjectIDWithoutMerge(t *testing.T) {
	c := newTestContainer()

	if got, absorbed := c.Add(&item.Instance{ObjectID: 0, TemplateID: daggerTemplateID, Count: 1}); got != nil || absorbed {
		t.Fatalf("Add() with zero object id = (%+v, %v), want nil,false", got, absorbed)
	}
	if c.ItemByObjectID(0) != nil {
		t.Fatal("container registered object id 0")
	}

	existing := c.AddNew(adenaTemplateID, 5, 0x20000001)
	got, absorbed := c.Add(&item.Instance{ObjectID: 0, TemplateID: adenaTemplateID, Count: 3})
	if got != existing || !absorbed {
		t.Fatalf("Add() zero-id stack merge = (%+v, %v), want existing,true", got, absorbed)
	}
	if existing.Count != 8 {
		t.Fatalf("merged count = %d, want 8", existing.Count)
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

func TestContainer_Transfer_UsesInventoryAddHooks(t *testing.T) {
	src := newTestContainer()
	dst := NewPlayerInventory(0x10000002, testTemplates())

	dst.AddNew(adenaTemplateID, 5, 0x20000002)
	dst.DrainUpdates()
	inst := src.AddNew(adenaTemplateID, 100, 0x20000001)

	result, freedID, freed := src.Transfer(inst.ObjectID, 40, dst, 0)
	if result == nil || result.Count != 45 {
		t.Fatalf("Transfer() result = %+v, want destination count 45", result)
	}
	if freed {
		t.Errorf("partial transfer reported freed id %d", freedID)
	}
	updates := dst.DrainUpdates()
	if len(updates) != 1 || updates[0].ObjectID != result.ObjectID || updates[0].State != UpdateModified || updates[0].Count != 45 {
		t.Fatalf("destination updates = %+v, want one MODIFIED update with count 45", updates)
	}
}

func TestContainer_Transfer_UsesNewObjectIDWhenTargetStackDisappears(t *testing.T) {
	src := newTestContainer()
	dst := &staleMergeTarget{Container: NewContainer(0x10000002, item.LocationWarehouse, testTemplates())}

	dst.AddNew(adenaTemplateID, 5, 0x20000002)
	inst := src.AddNew(adenaTemplateID, 100, 0x20000001)

	result, _, freed := src.Transfer(inst.ObjectID, 40, dst, 0x30000001)
	if result == nil {
		t.Fatalf("Transfer() returned nil")
	}
	if result.ObjectID != 0x30000001 {
		t.Fatalf("Transfer() result object id = %#x, want newObjectID", result.ObjectID)
	}
	if dst.ItemByObjectID(0) != nil {
		t.Fatalf("destination contains item with object id 0")
	}
	if freed {
		t.Errorf("partial transfer reported a freed source id")
	}
}

type staleMergeTarget struct {
	*Container
}

func (t *staleMergeTarget) ItemByTemplateID(templateID int32) *item.Instance {
	inst := t.Container.ItemByTemplateID(templateID)
	if inst != nil {
		t.DestroyAllItems()
	}
	return inst
}

func TestContainer_Transfer_UsesFreightVisibleTownHooks(t *testing.T) {
	src := NewContainer(0x10000001, item.LocationWarehouse, freightTestTemplates())
	dst := NewFreight(0x10000002, freightTestTemplates())

	dst.ActiveLocation = 1
	townOne := dst.AddNew(freightTestStackableID, 10, 0x20000002)
	dst.ActiveLocation = 2
	inst := src.AddNew(freightTestStackableID, 5, 0x20000001)

	result, _, freed := src.Transfer(inst.ObjectID, 5, dst, 0)
	if result == nil {
		t.Fatalf("Transfer() returned nil")
	}
	if result == townOne {
		t.Fatalf("Transfer() merged into a hidden town-1 stack")
	}
	if result.LocationData != 2 || result.Count != 5 {
		t.Errorf("transferred stack = count %d location %d, want count 5 location 2", result.Count, result.LocationData)
	}
	if freed {
		t.Errorf("whole-instance transfer should move the source instance, not free its object id")
	}
	if townOne.Count != 10 || townOne.LocationData != 1 {
		t.Errorf("hidden town-1 stack = count %d location %d, want count 10 location 1", townOne.Count, townOne.LocationData)
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

// ---- from freight_test.go ----
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

func TestFreight_VisibleItems_ZeroActiveLocationReturnsOnlyUntagged(t *testing.T) {
	f := NewFreight(0x10000001, freightTestTemplates())

	f.ActiveLocation = 1
	tagged := f.AddNew(freightTestItemID, 1, 0x20000001)
	f.ActiveLocation = 0
	untagged := f.AddNew(freightTestStackableID, 1, 0x20000002)

	visible := f.VisibleItems()
	if len(visible) != 1 {
		t.Fatalf("VisibleItems() at ActiveLocation=0 = %d items, want 1 (untagged only)", len(visible))
	}
	if visible[0] != untagged {
		t.Fatalf("VisibleItems() at ActiveLocation=0 returned object %d, want untagged %d", visible[0].ObjectID, untagged.ObjectID)
	}
	if got := f.VisibleSize(); got != 2 {
		t.Errorf("VisibleSize() at ActiveLocation=0 = %d, want 2 (zero-location wildcard)", got)
	}
	if got := f.ItemByTemplateID(freightTestItemID); got != tagged {
		t.Errorf("ItemByTemplateID(town-tagged) at ActiveLocation=0 = %v, want tagged item (zero-location wildcard)", got)
	}
	if got := f.ItemByTemplateID(freightTestStackableID); got != untagged {
		t.Errorf("ItemByTemplateID(untagged) at ActiveLocation=0 = %v, want untagged item", got)
	}
}

func TestFreight_VisibleItems_OrderedByObjectID(t *testing.T) {
	f := NewFreight(0x10000001, freightTestTemplates())
	f.ActiveLocation = 1

	for _, objectID := range []int32{0x20000003, 0x20000001, 0x20000004, 0x20000002} {
		f.AddNew(freightTestItemID, 1, objectID)
	}

	visible := f.VisibleItems()
	for i := 1; i < len(visible); i++ {
		if visible[i-1].ObjectID > visible[i].ObjectID {
			t.Fatalf("VisibleItems() object ids are not ordered: %d before %d", visible[i-1].ObjectID, visible[i].ObjectID)
		}
	}
}

func TestFreight_Add_MergesLowestObjectIDVisibleStack(t *testing.T) {
	f := NewFreight(0x10000001, freightTestTemplates())

	f.ActiveLocation = 1
	low := f.AddNew(freightTestStackableID, 10, 0x20000001)
	f.ActiveLocation = 2
	high := f.AddNew(freightTestStackableID, 5, 0x20000002)
	f.ActiveLocation = 0

	merged := f.AddNew(freightTestStackableID, 3, 0x20000003)
	if merged != low {
		t.Fatalf("AddNew() with multiple visible stacks returned object %d, want lowest visible object %d", merged.ObjectID, low.ObjectID)
	}
	if low.Count != 13 || high.Count != 5 {
		t.Errorf("stack counts after merge = low %d high %d, want low 13 high 5", low.Count, high.Count)
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

// ---- from persist_test.go ----
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

func TestInventoryRestoreNormalizesStacksAndEquipment(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: potionTemplateID, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
		{ID: twoHandID, Kind: item.KindWeapon, Slot: item.SlotLRHand, Weapon: &item.WeaponDetail{Type: item.WeaponBigSword}},
		{ID: shieldID, Kind: item.KindArmor, Slot: item.SlotLHand, Armor: &item.ArmorDetail{Type: item.ArmorShield}},
	})
	shield := &item.Instance{ObjectID: 0x20000001, TemplateID: shieldID, Count: 1, Location: item.LocationPaperdoll, LocationData: LHand, ManaLeft: -1}
	twoHand := &item.Instance{ObjectID: 0x20000002, TemplateID: twoHandID, Count: 1, Location: item.LocationPaperdoll, LocationData: RHand, ManaLeft: -1}
	inv := RestorePlayerInventory(0x10000001, templates, []*item.Instance{
		{ObjectID: 0x20000003, TemplateID: potionTemplateID, Count: 4, Location: item.LocationInventory, ManaLeft: -1},
		{ObjectID: 0x20000004, TemplateID: potionTemplateID, Count: 6, Location: item.LocationInventory, ManaLeft: -1},
		shield,
		twoHand,
	})

	if got := inv.ItemCount(potionTemplateID, -1, true); got != 10 {
		t.Errorf("restored potion count = %d, want 10", got)
	}
	if got := inv.Size(); got != 3 {
		t.Errorf("restored size = %d, want 3", got)
	}
	if got := inv.ItemAt(RHand); got != twoHand {
		t.Errorf("RHand = %v, want two-handed weapon", got)
	}
	if got := inv.ItemAt(LHand); got != nil {
		t.Errorf("LHand = %v, want nil", got)
	}
	if got := shield.Snapshot().Location; got != item.LocationInventory {
		t.Errorf("displaced shield location = %v, want inventory", got)
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
