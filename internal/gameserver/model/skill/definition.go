package skill

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// ID identifies a skill, independent of its level.
type ID int32

// Ref points at a specific level of a skill.
type Ref struct {
	ID    ID
	Level int
}

// heroSkillIDs are the skills a character earns by holding hero status.
var heroSkillIDs = map[ID]bool{395: true, 396: true, 1374: true, 1375: true, 1376: true}

// offensiveSkillTypes are the raw "skillType" tags whose effect is
// inherently harmful to the target, used to default Definition.Offensive
// when a level's data doesn't say so explicitly.
var offensiveSkillTypes = map[string]bool{
	"PDAM": true, "MDAM": true, "CPDAMPERCENT": true, "DOT": true, "BLEED": true,
	"POISON": true, "AGGDAMAGE": true, "DEBUFF": true, "AGGDEBUFF": true, "STUN": true,
	"ROOT": true, "CONFUSION": true, "ERASE": true, "BLOW": true, "FATAL": true,
	"FEAR": true, "DRAIN": true, "SLEEP": true, "CHARGEDAM": true, "DEATHLINK": true,
	"MANADAM": true, "MDOT": true, "MUTE": true, "SOULSHOT": true, "SPIRITSHOT": true,
	"SPOIL": true, "WEAKNESS": true, "SWEEP": true, "PARALYZE": true, "DRAIN_SOUL": true,
	"AGGREDUCE": true, "CANCEL": true, "MAGE_BANE": true, "WARRIOR_BANE": true,
	"AGGREMOVE": true, "AGGREDUCE_CHAR": true, "BEAST_FEED": true, "BETRAY": true,
	"DELUXE_KEY_UNLOCK": true, "SOW": true, "HARVEST": true, "INSTANT_JUMP": true,
}

// Definition holds one level's static data for one skill: its timing,
// consumption, range, and the raw classification tags (skill/effect type,
// formula stat key) that a not-yet-built effect engine interprets to decide
// what casting the skill actually does. This type carries that data without
// acting on it — building the effect itself, evaluating its conditions, and
// resolving a weapon-type restriction to an equipment mask are some other
// system's job.
type Definition struct {
	ID    ID
	Level int

	Name       string
	Activation Activation
	Magic      bool
	Potion     bool

	MPConsume        int
	MPInitialConsume int
	HPConsume        int

	TargetConsumeCount int
	TargetConsumeID    int
	ItemConsumeCount   int
	ItemConsumeID      int

	CastRange   int
	EffectRange int

	AbnormalLevel       int
	EffectAbnormalLevel int
	NegateLevel         int

	HitTime  int
	CoolTime int

	ReuseDelay  int
	EquipDelay  int
	SharedReuse *Ref

	Radius int

	Target Target
	Power  float32

	Attribute   string
	NegateTypes []string
	NegateIDs   []int

	MaxNegatedEffects int
	MagicLevel        int
	LevelDepend       int

	IgnoreResists bool
	StaticReuse   bool
	StaticHitTime bool

	// Stat names the formula key a func/condition attached elsewhere reads
	// or writes; interpreting it is that engine's job, not this type's.
	Stat string

	IgnoreShield bool

	// SkillType and EffectType are the raw effect-classification tags a
	// skill and its (optional, separately classified) side effect carry.
	// Interpreting them — building the actual damage/buff/debuff behavior —
	// belongs to the not-yet-built effect engine.
	SkillType  string
	EffectType string

	EffectID    int
	EffectPower int
	EffectLevel int

	Element      Element
	BaseLandRate int

	Overhit          bool
	KillByDOT        bool
	SuicideAttack    bool
	SiegeSummonSkill bool

	// WeaponsAllowed is the raw comma-separated weapon/armor type list a
	// level restricts casting to, or "" when unrestricted. Resolving a name
	// to its equipment mask is the item-type data's job, not this loader's.
	WeaponsAllowed string

	NextActionIsAttack bool
	MinPledgeClass     int

	TriggeredID      int
	TriggeredLevel   int
	ChanceType       string
	ActivationChance int

	Debuff     bool
	Offensive  bool
	MaxCharges int
	NumCharges int

	// HeroSkill reports whether holding hero status grants this skill,
	// independent of anything the level's own data says.
	HeroSkill bool

	BaseCritRate  int
	LethalChance1 int
	LethalChance2 int

	DirectHPDamage bool
	Dance          bool
	NextDanceCost  int
	SoulShotBoost  float32
	AggroPoints    int

	StayAfterDeath bool

	Flight    *Flight
	FlyRadius int
	FlyCourse float32

	Feed int

	CanBeReflected bool
	CanBeDispelled bool
	ClanSkill      bool

	SimultaneousCast bool

	// ExtractableItems is the raw, unparsed product list a "capsule" skill
	// (one that unwraps into a random item) carries, or "" when the skill
	// isn't one. Structuring it into item/quantity/chance rows is deferred
	// until something consumes it.
	ExtractableItems string
}

// NewDefinition builds one level's Definition from id, level, name (the
// <skill> element's own id/name, shared by every level) and set, the
// resolved attributes of that specific level. name, target, skillType and
// operateType are required; every other attribute defaults the way an
// absent one does in the shipped data.
func NewDefinition(id ID, level int, name string, set *commons.StatSet) (Definition, error) {
	wrap := func(err error) error { return fmt.Errorf("skill %d level %d: %w", id, level, err) }

	d := Definition{ID: id, Level: level, Name: name, HeroSkill: heroSkillIDs[id]}

	var err error
	if d.Activation, err = commons.GetEnum[Activation](set, "operateType", activationNames); err != nil {
		return Definition{}, wrap(err)
	}
	d.Magic = set.GetBoolDefault("isMagic", false)
	d.Potion = set.GetBoolDefault("isPotion", false)

	if d.MPConsume, err = set.GetIntDefault("mpConsume", 0); err != nil {
		return Definition{}, wrap(err)
	}
	if d.MPInitialConsume, err = set.GetIntDefault("mpInitialConsume", 0); err != nil {
		return Definition{}, wrap(err)
	}
	if d.HPConsume, err = set.GetIntDefault("hpConsume", 0); err != nil {
		return Definition{}, wrap(err)
	}
	if d.TargetConsumeCount, err = set.GetIntDefault("targetConsumeCount", 0); err != nil {
		return Definition{}, wrap(err)
	}
	if d.TargetConsumeID, err = set.GetIntDefault("targetConsumeId", 0); err != nil {
		return Definition{}, wrap(err)
	}
	if d.ItemConsumeCount, err = set.GetIntDefault("itemConsumeCount", 0); err != nil {
		return Definition{}, wrap(err)
	}
	if d.ItemConsumeID, err = set.GetIntDefault("itemConsumeId", 0); err != nil {
		return Definition{}, wrap(err)
	}
	if d.CastRange, err = set.GetIntDefault("castRange", 0); err != nil {
		return Definition{}, wrap(err)
	}
	if d.EffectRange, err = set.GetIntDefault("effectRange", -1); err != nil {
		return Definition{}, wrap(err)
	}
	if d.AbnormalLevel, err = set.GetIntDefault("abnormalLvl", -1); err != nil {
		return Definition{}, wrap(err)
	}
	if d.EffectAbnormalLevel, err = set.GetIntDefault("effectAbnormalLvl", -1); err != nil {
		return Definition{}, wrap(err)
	}
	if d.NegateLevel, err = set.GetIntDefault("negateLvl", -1); err != nil {
		return Definition{}, wrap(err)
	}
	if d.HitTime, err = set.GetIntDefault("hitTime", 0); err != nil {
		return Definition{}, wrap(err)
	}
	if d.CoolTime, err = set.GetIntDefault("coolTime", 0); err != nil {
		return Definition{}, wrap(err)
	}
	if d.ReuseDelay, err = set.GetIntDefault("reuseDelay", 0); err != nil {
		return Definition{}, wrap(err)
	}
	if d.EquipDelay, err = set.GetIntDefault("equipDelay", 0); err != nil {
		return Definition{}, wrap(err)
	}
	if set.Has("sharedReuse") {
		raw, err := set.GetString("sharedReuse")
		if err != nil {
			return Definition{}, wrap(err)
		}
		ref, err := parseSharedReuse(raw)
		if err != nil {
			return Definition{}, wrap(fmt.Errorf("sharedReuse %q: %w", raw, err))
		}
		d.SharedReuse = &ref
	}
	if d.Radius, err = set.GetIntDefault("skillRadius", 80); err != nil {
		return Definition{}, wrap(err)
	}

	if d.Target, err = commons.GetEnum[Target](set, "target", targetNames); err != nil {
		return Definition{}, wrap(err)
	}
	if d.Power, err = set.GetFloat32Default("power", 0); err != nil {
		return Definition{}, wrap(err)
	}

	d.Attribute = set.GetStringDefault("attribute", "")
	if negate := set.GetStringDefault("negateStats", ""); negate != "" {
		d.NegateTypes = strings.Fields(negate)
	}
	if set.Has("negateId") {
		raw, err := set.GetString("negateId")
		if err != nil {
			return Definition{}, wrap(err)
		}
		d.NegateIDs, err = parseCommaInts(raw)
		if err != nil {
			return Definition{}, wrap(fmt.Errorf("negateId %q: %w", raw, err))
		}
	}

	if d.MaxNegatedEffects, err = set.GetIntDefault("maxNegated", 0); err != nil {
		return Definition{}, wrap(err)
	}
	if d.MagicLevel, err = set.GetIntDefault("magicLvl", 0); err != nil {
		return Definition{}, wrap(err)
	}
	if d.LevelDepend, err = set.GetIntDefault("lvlDepend", 0); err != nil {
		return Definition{}, wrap(err)
	}
	d.IgnoreResists = set.GetBoolDefault("ignoreResists", false)
	d.StaticReuse = set.GetBoolDefault("staticReuse", false)
	d.StaticHitTime = set.GetBoolDefault("staticHitTime", false)

	d.Stat = set.GetStringDefault("stat", "")
	d.IgnoreShield = set.GetBoolDefault("ignoreShld", false)

	if d.SkillType, err = set.GetString("skillType"); err != nil {
		return Definition{}, wrap(err)
	}
	d.EffectType = set.GetStringDefault("effectType", "")

	if d.EffectID, err = set.GetIntDefault("effectId", 0); err != nil {
		return Definition{}, wrap(err)
	}
	if d.EffectPower, err = set.GetIntDefault("effectPower", 0); err != nil {
		return Definition{}, wrap(err)
	}
	if d.EffectLevel, err = set.GetIntDefault("effectLevel", 0); err != nil {
		return Definition{}, wrap(err)
	}

	if d.Element, err = commons.GetEnumDefault[Element](set, "element", elementNames, ElementNone); err != nil {
		return Definition{}, wrap(err)
	}
	if d.BaseLandRate, err = set.GetIntDefault("baseLandRate", 0); err != nil {
		return Definition{}, wrap(err)
	}

	d.Overhit = set.GetBoolDefault("overHit", false)
	d.KillByDOT = set.GetBoolDefault("killByDOT", false)
	d.SuicideAttack = set.GetBoolDefault("isSuicideAttack", false)
	d.SiegeSummonSkill = set.GetBoolDefault("isSiegeSummonSkill", false)

	d.WeaponsAllowed = set.GetStringDefault("weaponsAllowed", "")

	d.NextActionIsAttack = set.GetBoolDefault("nextActionAttack", false)
	if d.MinPledgeClass, err = set.GetIntDefault("minPledgeClass", 0); err != nil {
		return Definition{}, wrap(err)
	}

	if d.TriggeredID, err = set.GetIntDefault("triggeredId", 0); err != nil {
		return Definition{}, wrap(err)
	}
	if d.TriggeredLevel, err = set.GetIntDefault("triggeredLevel", 0); err != nil {
		return Definition{}, wrap(err)
	}
	d.ChanceType = set.GetStringDefault("chanceType", "")
	if d.ActivationChance, err = set.GetIntDefault("activationChance", -1); err != nil {
		return Definition{}, wrap(err)
	}

	d.Debuff = set.GetBoolDefault("isDebuff", false)
	d.Offensive = set.GetBoolDefault("offensive", isTypeOffensive(d.SkillType) || d.Debuff || d.Target == TargetCorpseMob)
	if d.MaxCharges, err = set.GetIntDefault("maxCharges", 0); err != nil {
		return Definition{}, wrap(err)
	}
	if d.NumCharges, err = set.GetIntDefault("numCharges", 0); err != nil {
		return Definition{}, wrap(err)
	}

	if d.BaseCritRate, err = set.GetIntDefault("baseCritRate", defaultBaseCritRate(d.SkillType)); err != nil {
		return Definition{}, wrap(err)
	}
	if d.LethalChance1, err = set.GetIntDefault("lethal1", 0); err != nil {
		return Definition{}, wrap(err)
	}
	if d.LethalChance2, err = set.GetIntDefault("lethal2", 0); err != nil {
		return Definition{}, wrap(err)
	}

	d.DirectHPDamage = set.GetBoolDefault("dmgDirectlyToHp", false)
	d.Dance = set.GetBoolDefault("isDance", false)
	if d.NextDanceCost, err = set.GetIntDefault("nextDanceCost", 0); err != nil {
		return Definition{}, wrap(err)
	}
	if d.SoulShotBoost, err = set.GetFloat32Default("SSBoost", 0); err != nil {
		return Definition{}, wrap(err)
	}
	if d.AggroPoints, err = set.GetIntDefault("aggroPoints", 0); err != nil {
		return Definition{}, wrap(err)
	}

	d.StayAfterDeath = set.GetBoolDefault("stayAfterDeath", false)

	if set.Has("flyType") {
		f, err := commons.GetEnum[Flight](set, "flyType", flightNames)
		if err != nil {
			return Definition{}, wrap(err)
		}
		d.Flight = &f
	}
	if d.FlyRadius, err = set.GetIntDefault("flyRadius", 0); err != nil {
		return Definition{}, wrap(err)
	}
	if d.FlyCourse, err = set.GetFloat32Default("flyCourse", 0); err != nil {
		return Definition{}, wrap(err)
	}

	if d.Feed, err = set.GetIntDefault("feed", 0); err != nil {
		return Definition{}, wrap(err)
	}

	d.CanBeReflected = set.GetBoolDefault("canBeReflected", true)
	d.CanBeDispelled = set.GetBoolDefault("canBeDispeled", true)
	d.ClanSkill = set.GetBoolDefault("isClanSkill", false)
	d.SimultaneousCast = set.GetBoolDefault("simultaneousCast", false)

	d.ExtractableItems = set.GetStringDefault("capsuled_items_skill", "")

	return d, nil
}

// isTypeOffensive reports whether skillType is one of the raw effect tags
// that is inherently harmful to its target, used to default a level's
// Offensive field when its data doesn't say so explicitly.
func isTypeOffensive(skillType string) bool {
	return offensiveSkillTypes[skillType]
}

// defaultBaseCritRate is the BaseCritRate a level defaults to when its data
// doesn't set one explicitly: a physical-damage or blow skill always has a
// chance to critical, everything else has none.
func defaultBaseCritRate(skillType string) int {
	if skillType == "PDAM" || skillType == "BLOW" {
		return 0
	}
	return -1
}

// parseDashPair parses a "left-right" pair of integers, the shape a few
// unrelated attributes across this package share (a shared-reuse skill
// reference, a required-item id and count). left is always an id and so is
// parsed with a 32-bit bound; right is a plain count/level.
func parseDashPair(raw string) (int32, int, error) {
	parts := strings.Split(raw, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("want \"left-right\"")
	}
	left, err := strconv.ParseInt(parts[0], 10, 32)
	if err != nil {
		return 0, 0, err
	}
	right, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, err
	}
	return int32(left), right, nil
}

// parseSharedReuse parses a "sharedReuse" attribute's "skillId-level" form.
func parseSharedReuse(raw string) (Ref, error) {
	id, level, err := parseDashPair(raw)
	if err != nil {
		return Ref{}, err
	}
	return Ref{ID: ID(id), Level: level}, nil
}

// parseCommaInts parses a comma-separated list of integers.
func parseCommaInts(raw string) ([]int, error) {
	parts := strings.Split(raw, ",")
	out := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, err
		}
		out[i] = n
	}
	return out, nil
}
