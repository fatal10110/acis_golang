package itemcontainer

import (
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

type inventoryDeliveryRecorder struct {
	updates int
	weights int
}

func (r *inventoryDeliveryRecorder) QueueInventoryUpdate(*Inventory) { r.updates++ }

func (r *inventoryDeliveryRecorder) UpdateInventoryWeight(*Inventory) { r.weights++ }

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
