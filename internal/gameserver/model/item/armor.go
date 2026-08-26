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

// Mask returns the worn-item bit a occupies in an inventory's worn-type
// mask. ArmorType's bits sit immediately above WeaponType's so the two
// families share one bitmask space without colliding.
func (a ArmorType) Mask() int32 {
	return 1 << (uint(a) + uint(weaponTypeCount))
}

// ParseArmorType resolves a template's "armor_type" attribute to an
// ArmorType. It returns an error for any value outside the shipped set
// rather than guessing.
func ParseArmorType(s string) (ArmorType, error) {
	a, ok := armorTypeNames[s]
	if !ok {
		return 0, fmt.Errorf("item: unknown armor type %q", s)
	}
	return a, nil
}

// ArmorDetail is the armor-specific data a KindArmor Template carries; nil
// for every other Kind.
type ArmorDetail struct {
	Type ArmorType
}

// NewArmorDetail builds the ArmorDetail for a KindArmor template declaring
// armorType and occupying slot. An unspecified armor_type worn in the
// one-handed slot reports as a shield: the shipped data leaves shields
// untyped and relies on the slot alone to distinguish them.
func NewArmorDetail(armorType ArmorType, slot Slot) *ArmorDetail {
	if armorType == ArmorNone && slot == SlotLHand {
		armorType = ArmorShield
	}
	return &ArmorDetail{Type: armorType}
}
