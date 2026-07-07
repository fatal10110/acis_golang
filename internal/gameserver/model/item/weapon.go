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

var weaponTypeNames = reverseStringMap(weaponTypeStrings)

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
	d := &WeaponDetail{}
	var err error

	if d.Type, err = commons.GetEnumDefault(set, "weapon_type", weaponTypeNames, WeaponNone); err != nil {
		return nil, fmt.Errorf("item: weapon: %w", err)
	}
	if d.SoulshotCount, err = set.GetInt32Default("soulshots", 0); err != nil {
		return nil, fmt.Errorf("item: weapon: %w", err)
	}
	if d.SpiritshotCount, err = set.GetInt32Default("spiritshots", 0); err != nil {
		return nil, fmt.Errorf("item: weapon: %w", err)
	}
	if d.RandomDamage, err = set.GetInt32Default("random_damage", 0); err != nil {
		return nil, fmt.Errorf("item: weapon: %w", err)
	}
	if d.MPConsume, err = set.GetInt32Default("mp_consume", 0); err != nil {
		return nil, fmt.Errorf("item: weapon: %w", err)
	}

	rate, value, err := parseIntPairDefault(set, "mp_consume_reduce", 0, 0)
	if err != nil {
		return nil, fmt.Errorf("item: weapon: %w", err)
	}
	d.MPConsumeReduceRate, d.MPConsumeReduceValue = rate, value

	if d.ReuseDelay, err = set.GetInt32Default("reuse_delay", 0); err != nil {
		return nil, fmt.Errorf("item: weapon: %w", err)
	}
	d.Magical = set.GetBoolDefault("is_magical", false)

	chance, count, err := parseIntPairDefault(set, "reduced_soulshot", 0, 0)
	if err != nil {
		return nil, fmt.Errorf("item: weapon: %w", err)
	}
	d.ReducedSoulshotChance, d.ReducedSoulshotCount = chance, count

	if set.Has("enchant4_skill") {
		s, err := set.GetString("enchant4_skill")
		if err != nil {
			return nil, fmt.Errorf("item: weapon: %w", err)
		}
		ref, err := ParseSkillRef(s)
		if err != nil {
			return nil, fmt.Errorf("item: weapon: %w", err)
		}
		d.Enchant4Skill = &ref
	}

	d.OnCastSkill, err = parseSkillTrigger(set, "oncast_skill", "oncast_chance")
	if err != nil {
		return nil, fmt.Errorf("item: weapon: %w", err)
	}
	d.OnCritSkill, err = parseSkillTrigger(set, "oncrit_skill", "oncrit_chance")
	if err != nil {
		return nil, fmt.Errorf("item: weapon: %w", err)
	}

	return d, nil
}

// parseSkillTrigger reads the optional (skillKey, chanceKey) pair a weapon
// uses to describe an on-cast/on-crit triggered skill: skillKey is an
// "id-level" SkillRef, chanceKey an optional percentage read only when
// skillKey is present (matching the shipped data's own contract: a chance
// value with no skill to gate is never read at all). Returns nil, nil when
// skillKey is absent.
func parseSkillTrigger(set *commons.StatSet, skillKey, chanceKey string) (*SkillTrigger, error) {
	if !set.Has(skillKey) {
		return nil, nil
	}
	s, err := set.GetString(skillKey)
	if err != nil {
		return nil, err
	}
	ref, err := ParseSkillRef(s)
	if err != nil {
		return nil, err
	}

	chance := int32(-1)
	if set.Has(chanceKey) {
		if chance, err = set.GetInt32(chanceKey); err != nil {
			return nil, err
		}
	}
	return &SkillTrigger{Skill: ref, Chance: chance}, nil
}

// parseIntPairDefault reads key as an "a,b" pair of int32s, returning
// (defaultA, defaultB) when key is absent. A present value that isn't
// exactly two comma-separated integers is an error.
func parseIntPairDefault(set *commons.StatSet, key string, defaultA, defaultB int32) (int32, int32, error) {
	if !set.Has(key) {
		return defaultA, defaultB, nil
	}
	raw, err := set.GetString(key)
	if err != nil {
		return 0, 0, err
	}
	parts := strings.Split(raw, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("item: attribute %q: want \"a,b\", got %q", key, raw)
	}
	a, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("item: attribute %q: %w", key, err)
	}
	b, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("item: attribute %q: %w", key, err)
	}
	return int32(a), int32(b), nil
}
