package item

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// ArmorType is the protection class a KindArmor template belongs to, driving
// which defense formula applies and (for LIGHT/HEAVY/MAGIC) which class
// restrictions the client enforces.
type ArmorType uint8

const (
	ArmorNone ArmorType = iota
	ArmorLight
	ArmorHeavy
	ArmorMagic
	ArmorPet
	ArmorShield
)

// String returns the canonical XML spelling for a.
func (a ArmorType) String() string {
	name, ok := armorTypeStrings[a]
	if !ok {
		return fmt.Sprintf("ArmorType(%d)", uint8(a))
	}
	return name
}

var armorTypeStrings = map[ArmorType]string{
	ArmorNone:   "NONE",
	ArmorLight:  "LIGHT",
	ArmorHeavy:  "HEAVY",
	ArmorMagic:  "MAGIC",
	ArmorPet:    "PET",
	ArmorShield: "SHIELD",
}

var armorTypeNames = commons.ReverseMap(armorTypeStrings)

// ArmorDetail is the armor-specific data a KindArmor Template carries; nil
// for every other Kind.
type ArmorDetail struct {
	Type ArmorType
}

// NewArmorDetail builds an ArmorDetail from set, the template's folded
// top-level attributes, and slot, the template's own equip slot. An
// unspecified armor_type worn in the one-handed slot reports as a shield:
// the shipped data leaves shields untyped and relies on the slot alone to
// distinguish them.
func NewArmorDetail(set *commons.StatSet, slot Slot) (*ArmorDetail, error) {
	armorType, err := commons.GetEnumDefault(set, "armor_type", armorTypeNames, ArmorNone)
	if err != nil {
		return nil, fmt.Errorf("item: armor: %w", err)
	}
	if armorType == ArmorNone && slot == SlotLHand {
		armorType = ArmorShield
	}
	return &ArmorDetail{Type: armorType}, nil
}
