package itemcontainer

import (
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

func TestInventory_EquipItem_TwoHandedClearsOffhand(t *testing.T) {
	f := newEquipFixture()
	shield, _ := f.equip(shieldID)
	if f.inv.ItemAt(LHand) != shield {
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

func TestInventory_EquipItem_OneHandedClearsExistingTwoHanded(t *testing.T) {
	f := newEquipFixture()
	f.equip(twoHandID)

	sword, _ := f.equip(swordID)
	if f.inv.ItemAt(RHand) != sword {
		t.Errorf("one-handed sword should now occupy RHand")
	}
}

func TestInventory_EquipItem_BowArrowPairingKeepsOffhand(t *testing.T) {
	f := newEquipFixture()
	f.equip(bowID)

	arrow, _ := f.equip(arrowID)
	if f.inv.ItemAt(LHand) != arrow {
		t.Fatalf("arrow should occupy LHand")
	}
	if f.inv.ItemAt(RHand) == nil {
		t.Errorf("equipping an arrow while a bow is worn must not clear the bow")
	}
}

func TestInventory_EquipItem_FishingRodLurePairingKeepsOffhand(t *testing.T) {
	f := newEquipFixture()
	f.equip(rodID)

	f.equip(lureID)
	if f.inv.ItemAt(RHand) == nil {
		t.Errorf("equipping a lure while a fishing rod is worn must not clear the rod")
	}
}

func TestInventory_EquipItem_LHandClearsTwoHandedWhenNotPaired(t *testing.T) {
	f := newEquipFixture()
	f.equip(twoHandID)

	f.equip(shieldID)
	if f.inv.ItemAt(RHand) != nil {
		t.Errorf("equipping a shield (unpaired LHand item) while two-handed should clear RHand")
	}
}

func TestInventory_EquipItem_EarsFillFirstEmptyThenReplaceLeft(t *testing.T) {
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
}

func TestInventory_EquipItem_FingersSameShape(t *testing.T) {
	f := newEquipFixture()
	first, _ := f.equip(ringID)
	if f.inv.ItemAt(LFinger) != first {
		t.Fatalf("first ring should fill LFinger")
	}
	second, _ := f.equip(ringID)
	if f.inv.ItemAt(RFinger) != second {
		t.Fatalf("second ring should fill RFinger")
	}
}

func TestInventory_EquipItem_FullArmorClearsLegs(t *testing.T) {
	f := newEquipFixture()
	f.equip(legsLightID)

	full, _ := f.equip(fullArmorID)
	if f.inv.ItemAt(Chest) != full {
		t.Fatalf("full armor should occupy Chest")
	}
	if f.inv.ItemAt(Legs) != nil {
		t.Errorf("equipping full armor should clear Legs")
	}
}

func TestInventory_EquipItem_LegsClearsFullArmor(t *testing.T) {
	f := newEquipFixture()
	f.equip(fullArmorID)

	legs, _ := f.equip(legsLightID)
	if f.inv.ItemAt(Legs) != legs {
		t.Fatalf("legs should occupy Legs")
	}
	if f.inv.ItemAt(Chest) != nil {
		t.Errorf("equipping legs while full armor is worn should clear Chest")
	}
}

func TestInventory_EquipItem_AllDressClearsSixSlots(t *testing.T) {
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
}

func TestInventory_EquipItem_HairAllClearsFaceAndViceVersa(t *testing.T) {
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
	legsTmpl, _ := f.templates.Get(legsLightID)

	f.equip(chestLightID)
	f.equip(legsLightID)
	if !f.inv.IsWearingType(chestTmpl.Mask()) {
		t.Errorf("matching light chest+legs should register the light-armor worn mask")
	}
	_ = legsTmpl

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
