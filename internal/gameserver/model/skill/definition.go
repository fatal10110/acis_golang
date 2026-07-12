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

	Conditions  []ConditionClause
	Funcs       []FuncTemplate
	Effects     []EffectTemplate
	SelfEffects []EffectTemplate
}

// NewDefinition builds one level's Definition from id, level, name (the
// <skill> element's own id/name, shared by every level) and set, the
// resolved attributes of that specific level. name, target, skillType and
// operateType are required; every other attribute defaults the way an
// absent one does in the shipped data.
func NewDefinition(id ID, level int, name string, set *commons.StatSet) (Definition, error) {
	f := commons.NewFields(set, fmt.Sprintf("skill %d level %d", id, level))

	d := Definition{
		ID: id, Level: level, Name: name, HeroSkill: heroSkillIDs[id],

		Activation: commons.FieldEnum[Activation](f, "operateType", activationNames),
		Magic:      f.BoolDefault("isMagic", false),
		Potion:     f.BoolDefault("isPotion", false),

		MPConsume:        f.IntDefault("mpConsume", 0),
		MPInitialConsume: f.IntDefault("mpInitialConsume", 0),
		HPConsume:        f.IntDefault("hpConsume", 0),

		TargetConsumeCount: f.IntDefault("targetConsumeCount", 0),
		TargetConsumeID:    f.IntDefault("targetConsumeId", 0),
		ItemConsumeCount:   f.IntDefault("itemConsumeCount", 0),
		ItemConsumeID:      f.IntDefault("itemConsumeId", 0),

		CastRange:           f.IntDefault("castRange", 0),
		EffectRange:         f.IntDefault("effectRange", -1),
		AbnormalLevel:       f.IntDefault("abnormalLvl", -1),
		EffectAbnormalLevel: f.IntDefault("effectAbnormalLvl", -1),
		NegateLevel:         f.IntDefault("negateLvl", -1),

		HitTime:    f.IntDefault("hitTime", 0),
		CoolTime:   f.IntDefault("coolTime", 0),
		ReuseDelay: f.IntDefault("reuseDelay", 0),
		EquipDelay: f.IntDefault("equipDelay", 0),

		Radius: f.IntDefault("skillRadius", 80),

		Target: commons.FieldEnum[Target](f, "target", targetNames),
		Power:  f.Float32Default("power", 0),

		Attribute: f.StringDefault("attribute", ""),

		MaxNegatedEffects: f.IntDefault("maxNegated", 0),
		MagicLevel:        f.IntDefault("magicLvl", 0),
		LevelDepend:       f.IntDefault("lvlDepend", 0),
		IgnoreResists:     f.BoolDefault("ignoreResists", false),
		StaticReuse:       f.BoolDefault("staticReuse", false),
		StaticHitTime:     f.BoolDefault("staticHitTime", false),

		Stat:         f.StringDefault("stat", ""),
		IgnoreShield: f.BoolDefault("ignoreShld", false),

		SkillType:  f.String("skillType"),
		EffectType: f.StringDefault("effectType", ""),

		EffectID:    f.IntDefault("effectId", 0),
		EffectPower: f.IntDefault("effectPower", 0),
		EffectLevel: f.IntDefault("effectLevel", 0),

		Element:      commons.FieldEnumDefault[Element](f, "element", elementNames, ElementNone),
		BaseLandRate: f.IntDefault("baseLandRate", 0),

		Overhit:          f.BoolDefault("overHit", false),
		KillByDOT:        f.BoolDefault("killByDOT", false),
		SuicideAttack:    f.BoolDefault("isSuicideAttack", false),
		SiegeSummonSkill: f.BoolDefault("isSiegeSummonSkill", false),

		WeaponsAllowed: f.StringDefault("weaponsAllowed", ""),

		NextActionIsAttack: f.BoolDefault("nextActionAttack", false),
		MinPledgeClass:     f.IntDefault("minPledgeClass", 0),

		TriggeredID:      f.IntDefault("triggeredId", 0),
		TriggeredLevel:   f.IntDefault("triggeredLevel", 0),
		ChanceType:       f.StringDefault("chanceType", ""),
		ActivationChance: f.IntDefault("activationChance", -1),

		Debuff:     f.BoolDefault("isDebuff", false),
		MaxCharges: f.IntDefault("maxCharges", 0),
		NumCharges: f.IntDefault("numCharges", 0),

		LethalChance1: f.IntDefault("lethal1", 0),
		LethalChance2: f.IntDefault("lethal2", 0),

		DirectHPDamage: f.BoolDefault("dmgDirectlyToHp", false),
		Dance:          f.BoolDefault("isDance", false),
		NextDanceCost:  f.IntDefault("nextDanceCost", 0),
		SoulShotBoost:  f.Float32Default("SSBoost", 0),
		AggroPoints:    f.IntDefault("aggroPoints", 0),

		StayAfterDeath: f.BoolDefault("stayAfterDeath", false),

		FlyRadius: f.IntDefault("flyRadius", 0),
		FlyCourse: f.Float32Default("flyCourse", 0),

		Feed: f.IntDefault("feed", 0),

		CanBeReflected:   f.BoolDefault("canBeReflected", true),
		CanBeDispelled:   f.BoolDefault("canBeDispeled", true),
		ClanSkill:        f.BoolDefault("isClanSkill", false),
		SimultaneousCast: f.BoolDefault("simultaneousCast", false),

		ExtractableItems: f.StringDefault("capsuled_items_skill", ""),
	}

	if negate := f.StringDefault("negateStats", ""); negate != "" {
		d.NegateTypes = strings.Fields(negate)
	}

	if f.Has("sharedReuse") {
		raw := f.String("sharedReuse")
		ref, err := parseSharedReuse(raw)
		if err != nil {
			f.Fail(fmt.Errorf("sharedReuse %q: %w", raw, err))
		} else {
			d.SharedReuse = &ref
		}
	}

	if f.Has("negateId") {
		raw := f.String("negateId")
		ids, err := parseCommaInts(raw)
		if err != nil {
			f.Fail(fmt.Errorf("negateId %q: %w", raw, err))
		} else {
			d.NegateIDs = ids
		}
	}

	d.Offensive = f.BoolDefault("offensive", isTypeOffensive(d.SkillType) || d.Debuff || d.Target == TargetCorpseMob)
	d.BaseCritRate = f.IntDefault("baseCritRate", defaultBaseCritRate(d.SkillType))

	if f.Has("flyType") {
		flight := commons.FieldEnum[Flight](f, "flyType", flightNames)
		d.Flight = &flight
	}

	if err := f.Err(); err != nil {
		return Definition{}, err
	}
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
