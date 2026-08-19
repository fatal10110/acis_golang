package skill

import (
	"fmt"
	"strconv"
	"strings"
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

	// EffectNpcID is the template id of the world actor a signet-family
	// skill (SIGNET/SIGNET_CASTTIME) spawns to carry its periodic area
	// effect. -1 when unset.
	EffectNpcID int

	Element      Element
	BaseLandRate int

	Overhit          bool
	KillByDOT        bool
	SuicideAttack    bool
	SiegeSummonSkill bool

	// IsCubic and NpcID select the cubic branch of a SUMMON skill: NpcID is
	// the cubic type id (matching cubic.ID) the skill grants when IsCubic
	// is set. The same NpcID attribute also carries a servitor's spawn
	// template id when IsCubic is false, which is that branch's concern.
	IsCubic bool
	NpcID   int

	// CubicActivationTime and CubicActivationChance are a granting SUMMON
	// skill's own "activationtime" (seconds between action ticks) and
	// "activationchance" (percent roll gating each non-Life-Cubic tick),
	// distinct from the unrelated ChanceSkillTrigger effect's
	// "activationChance" attribute captured by ActivationChance above.
	// SummonTotalLifeTime is the granted cubic's total lifetime in ms.
	CubicActivationTime   int
	CubicActivationChance int
	SummonTotalLifeTime   int

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

// DefinitionAttrs holds one skill level's static attributes after the data
// loader has resolved each to a concrete Go value: a level's raw source is
// a per-level substitution of a shared attribute list, so there is no fixed
// element per attribute for this package to decode itself. Parsing and
// defaulting happen at the loader; this type is the typed handoff into
// NewDefinition, which adds only the fields derived from other fields.
//
// Offensive and BaseCritRate are pointers because their default is derived
// rather than fixed: nil means the level's data didn't set one, and
// NewDefinition fills it in from the level's other attributes.
type DefinitionAttrs struct {
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

	Stat string

	IgnoreShield bool

	SkillType  string
	EffectType string

	EffectID    int
	EffectPower int
	EffectLevel int
	EffectNpcID int

	Element      Element
	BaseLandRate int

	Overhit          bool
	KillByDOT        bool
	SuicideAttack    bool
	SiegeSummonSkill bool

	IsCubic bool
	NpcID   int

	CubicActivationTime   int
	CubicActivationChance int
	SummonTotalLifeTime   int

	WeaponsAllowed string

	NextActionIsAttack bool
	MinPledgeClass     int

	TriggeredID      int
	TriggeredLevel   int
	ChanceType       string
	ActivationChance int

	Debuff     bool
	MaxCharges int
	NumCharges int

	Offensive    *bool
	BaseCritRate *int

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

	ExtractableItems string
}

// NewDefinition builds one level's Definition from id, level, name (the
// skill's own id and name, shared by every level) and a, that specific
// level's already-resolved attributes. It adds the fields a level's own
// data never carries: hero-skill membership, and the Offensive and
// BaseCritRate defaults derived from the level's other attributes when a
// leaves them unset.
func NewDefinition(id ID, level int, name string, a DefinitionAttrs) Definition {
	d := Definition{
		ID: id, Level: level, Name: name, HeroSkill: heroSkillIDs[id],

		Activation: a.Activation,
		Magic:      a.Magic,
		Potion:     a.Potion,

		MPConsume:        a.MPConsume,
		MPInitialConsume: a.MPInitialConsume,
		HPConsume:        a.HPConsume,

		TargetConsumeCount: a.TargetConsumeCount,
		TargetConsumeID:    a.TargetConsumeID,
		ItemConsumeCount:   a.ItemConsumeCount,
		ItemConsumeID:      a.ItemConsumeID,

		CastRange:           a.CastRange,
		EffectRange:         a.EffectRange,
		AbnormalLevel:       a.AbnormalLevel,
		EffectAbnormalLevel: a.EffectAbnormalLevel,
		NegateLevel:         a.NegateLevel,

		HitTime:    a.HitTime,
		CoolTime:   a.CoolTime,
		ReuseDelay: a.ReuseDelay,
		EquipDelay: a.EquipDelay,

		SharedReuse: a.SharedReuse,

		Radius: a.Radius,

		Target: a.Target,
		Power:  a.Power,

		Attribute:   a.Attribute,
		NegateTypes: a.NegateTypes,
		NegateIDs:   a.NegateIDs,

		MaxNegatedEffects: a.MaxNegatedEffects,
		MagicLevel:        a.MagicLevel,
		LevelDepend:       a.LevelDepend,
		IgnoreResists:     a.IgnoreResists,
		StaticReuse:       a.StaticReuse,
		StaticHitTime:     a.StaticHitTime,

		Stat:         a.Stat,
		IgnoreShield: a.IgnoreShield,

		SkillType:  a.SkillType,
		EffectType: a.EffectType,

		EffectID:    a.EffectID,
		EffectPower: a.EffectPower,
		EffectLevel: a.EffectLevel,
		EffectNpcID: a.EffectNpcID,

		Element:      a.Element,
		BaseLandRate: a.BaseLandRate,

		Overhit:          a.Overhit,
		KillByDOT:        a.KillByDOT,
		SuicideAttack:    a.SuicideAttack,
		SiegeSummonSkill: a.SiegeSummonSkill,

		IsCubic: a.IsCubic,
		NpcID:   a.NpcID,

		CubicActivationTime:   a.CubicActivationTime,
		CubicActivationChance: a.CubicActivationChance,
		SummonTotalLifeTime:   a.SummonTotalLifeTime,

		WeaponsAllowed: a.WeaponsAllowed,

		NextActionIsAttack: a.NextActionIsAttack,
		MinPledgeClass:     a.MinPledgeClass,

		TriggeredID:      a.TriggeredID,
		TriggeredLevel:   a.TriggeredLevel,
		ChanceType:       a.ChanceType,
		ActivationChance: a.ActivationChance,

		Debuff:     a.Debuff,
		MaxCharges: a.MaxCharges,
		NumCharges: a.NumCharges,

		LethalChance1: a.LethalChance1,
		LethalChance2: a.LethalChance2,

		DirectHPDamage: a.DirectHPDamage,
		Dance:          a.Dance,
		NextDanceCost:  a.NextDanceCost,
		SoulShotBoost:  a.SoulShotBoost,
		AggroPoints:    a.AggroPoints,

		StayAfterDeath: a.StayAfterDeath,

		Flight:    a.Flight,
		FlyRadius: a.FlyRadius,
		FlyCourse: a.FlyCourse,

		Feed: a.Feed,

		CanBeReflected:   a.CanBeReflected,
		CanBeDispelled:   a.CanBeDispelled,
		ClanSkill:        a.ClanSkill,
		SimultaneousCast: a.SimultaneousCast,

		ExtractableItems: a.ExtractableItems,
	}

	d.Offensive = isTypeOffensive(d.SkillType) || d.Debuff || d.Target == TargetCorpseMob
	if a.Offensive != nil {
		d.Offensive = *a.Offensive
	}

	d.BaseCritRate = defaultBaseCritRate(d.SkillType)
	if a.BaseCritRate != nil {
		d.BaseCritRate = *a.BaseCritRate
	}

	return d
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

// ParseRef parses a skill reference in its "skillId-level" data-file form,
// the shape a "sharedReuse" attribute carries.
func ParseRef(raw string) (Ref, error) {
	id, level, ok := strings.Cut(raw, "-")
	if !ok {
		return Ref{}, fmt.Errorf("want \"skillId-level\"")
	}
	rawID, err := strconv.ParseInt(id, 10, 32)
	if err != nil {
		return Ref{}, err
	}
	lvl, err := strconv.Atoi(level)
	if err != nil {
		return Ref{}, err
	}
	return Ref{ID: ID(rawID), Level: lvl}, nil
}
