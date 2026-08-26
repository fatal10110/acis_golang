package item

import (
	"fmt"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// WeaponType is the weapon family a KindWeapon template belongs to, driving
// its attack animation, range, and which combat formulas apply.
type WeaponType uint8

const (
	WeaponNone WeaponType = iota
	WeaponSword
	WeaponBlunt
	WeaponDagger
	WeaponBow
	WeaponPole
	WeaponEtc
	WeaponFist
	WeaponDual
	WeaponDualFist
	WeaponBigSword
	WeaponFishingRod
	WeaponBigBlunt
	WeaponPet
	weaponTypeEnd
)

// String returns the canonical XML spelling for w.
func (w WeaponType) String() string {
	name, ok := weaponTypeStrings[w]
	if !ok {
		return fmt.Sprintf("WeaponType(%d)", uint8(w))
	}
	return name
}

var weaponTypeStrings = map[WeaponType]string{
	WeaponNone:       "NONE",
	WeaponSword:      "SWORD",
	WeaponBlunt:      "BLUNT",
	WeaponDagger:     "DAGGER",
	WeaponBow:        "BOW",
	WeaponPole:       "POLE",
	WeaponEtc:        "ETC",
	WeaponFist:       "FIST",
	WeaponDual:       "DUAL",
	WeaponDualFist:   "DUALFIST",
	WeaponBigSword:   "BIGSWORD",
	WeaponFishingRod: "FISHINGROD",
	WeaponBigBlunt:   "BIGBLUNT",
	WeaponPet:        "PET",
}

var weaponTypeNames = commons.ReverseMap(weaponTypeStrings)

// weaponTypeCount is the number of WeaponType members; ArmorType's worn-mask
// bits start immediately above the bits WeaponType occupies, so the two
// families share one bitmask space without colliding.
const weaponTypeCount = int(weaponTypeEnd)

// ParseWornKindMask resolves a skill <using kind="..."/> attribute — a
// comma-separated list of WeaponType and/or ArmorType names — to the OR of
// their worn-mask bits, for a direct intersect check against
// Inventory.wornMask. An unrecognized token contributes no bits, matching
// DocumentBase.parseUsingCondition's silent-skip behavior in the Java
// reference (a typoed kind name is logged there, never rejected).
func ParseWornKindMask(kind string) int32 {
	var mask int32
	for _, tok := range strings.Split(kind, ",") {
		name := strings.TrimSpace(tok)
		if w, ok := weaponTypeNames[name]; ok {
			mask |= w.Mask()
		}
		if a, ok := armorTypeNames[name]; ok {
			mask |= a.Mask()
		}
	}
	return mask
}

// ParseWeaponType resolves a template's "weapon_type" attribute to a
// WeaponType. It returns an error for any value outside the shipped set
// rather than guessing.
func ParseWeaponType(s string) (WeaponType, error) {
	w, ok := weaponTypeNames[s]
	if !ok {
		return 0, fmt.Errorf("item: unknown weapon type %q", s)
	}
	return w, nil
}

// Mask returns the worn-item bit w occupies in an inventory's worn-type
// mask.
func (w WeaponType) Mask() int32 {
	return 1 << uint(w)
}

// WeaponDetail is the weapon-specific data a KindWeapon Template carries;
// nil for every other Kind.
type WeaponDetail struct {
	Type WeaponType

	SoulshotCount   int32
	SpiritshotCount int32
	RandomDamage    int32

	MPConsume            int32
	MPConsumeReduceRate  int32
	MPConsumeReduceValue int32

	ReuseDelay int32
	Magical    bool

	ReducedSoulshotChance int32
	ReducedSoulshotCount  int32

	// Enchant4Skill is the passive skill granted while the weapon is
	// enchanted +4 or higher; nil when the template grants none.
	Enchant4Skill *SkillRef

	// OnCastSkill/OnCritSkill are the skills the weapon triggers on spell
	// cast / on critical hit; nil when the template attaches none.
	OnCastSkill *SkillTrigger
	OnCritSkill *SkillTrigger
}
