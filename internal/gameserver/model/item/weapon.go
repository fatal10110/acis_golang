package item

import (
	"fmt"
	"strconv"
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
const weaponTypeCount = 14

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

// NewWeaponDetail builds a WeaponDetail from set, the template's folded
// top-level attributes. Every field defaults to its shipped-data default
// when absent; a present-but-malformed value is always an error.
func NewWeaponDetail(set *commons.StatSet) (*WeaponDetail, error) {
	f := commons.NewFields(set, "item: weapon")
	d := &WeaponDetail{
		Type:            commons.FieldEnumDefault[WeaponType](f, "weapon_type", weaponTypeNames, WeaponNone),
		SoulshotCount:   f.Int32Default("soulshots", 0),
		SpiritshotCount: f.Int32Default("spiritshots", 0),
		RandomDamage:    f.Int32Default("random_damage", 0),
		MPConsume:       f.Int32Default("mp_consume", 0),
	}

	rate, value := parseIntPairDefault(f, "mp_consume_reduce", 0, 0)
	d.MPConsumeReduceRate, d.MPConsumeReduceValue = rate, value

	d.ReuseDelay = f.Int32Default("reuse_delay", 0)
	d.Magical = f.BoolDefault("is_magical", false)

	chance, count := parseIntPairDefault(f, "reduced_soulshot", 0, 0)
	d.ReducedSoulshotChance, d.ReducedSoulshotCount = chance, count

	if f.Has("enchant4_skill") {
		s := f.String("enchant4_skill")
		if ref, err := ParseSkillRef(s); err != nil {
			f.Fail(err)
		} else {
			d.Enchant4Skill = &ref
		}
	}

	d.OnCastSkill = parseSkillTrigger(f, "oncast_skill", "oncast_chance")
	d.OnCritSkill = parseSkillTrigger(f, "oncrit_skill", "oncrit_chance")

	if err := f.Err(); err != nil {
		return nil, err
	}

	return d, nil
}

// parseSkillTrigger reads the optional (skillKey, chanceKey) pair a weapon
// uses to describe an on-cast/on-crit triggered skill: skillKey is an
// "id-level" SkillRef, chanceKey an optional percentage read only when
// skillKey is present (matching the shipped data's own contract: a chance
// value with no skill to gate is never read at all). Returns nil, nil when
// skillKey is absent.
func parseSkillTrigger(f *commons.Fields, skillKey, chanceKey string) *SkillTrigger {
	if f.Err() != nil || !f.Has(skillKey) {
		return nil
	}
	s := f.String(skillKey)
	ref, err := ParseSkillRef(s)
	if err != nil {
		f.Fail(err)
		return nil
	}

	chance := int32(-1)
	if f.Has(chanceKey) {
		chance = f.Int32(chanceKey)
	}
	if f.Err() != nil {
		return nil
	}
	return &SkillTrigger{Skill: ref, Chance: chance}
}

// parseIntPairDefault reads key as an "a,b" pair of int32s, returning
// (defaultA, defaultB) when key is absent. A present value that isn't
// exactly two comma-separated integers is an error.
func parseIntPairDefault(f *commons.Fields, key string, defaultA, defaultB int32) (int32, int32) {
	if f.Err() != nil || !f.Has(key) {
		return defaultA, defaultB
	}
	raw := f.String(key)
	parts := strings.Split(raw, ",")
	if len(parts) != 2 {
		f.Fail(fmt.Errorf("item: attribute %q: want \"a,b\", got %q", key, raw))
		return 0, 0
	}
	a, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 32)
	if err != nil {
		f.Fail(fmt.Errorf("item: attribute %q: %w", key, err))
		return 0, 0
	}
	b, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 32)
	if err != nil {
		f.Fail(fmt.Errorf("item: attribute %q: %w", key, err))
		return 0, 0
	}
	return int32(a), int32(b)
}
