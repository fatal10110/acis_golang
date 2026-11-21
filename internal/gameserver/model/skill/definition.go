package skill

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// ID identifies a skill, independent of its level.
type ID int32

// Ref points at a specific level of a skill.
type Ref struct {
	ID    ID
	Level int
}

// KeyLevels is how many levels one skill id spans in a combined (id, level)
// key, id*KeyLevels+level: the skill table's key and a cast's reuse key.
const KeyLevels = 256

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
// consumption, range, and the raw classification tag (skill type) that the
// skill handlers and effects interpret to decide what casting the skill
// actually does. This type carries that data without acting on it —
// building the effect itself, evaluating its conditions, and resolving a
// weapon-type restriction to an equipment mask are some other system's job.
//
// The attributes only some skill types carry live in Extra, reached through
// the Extra method. A loaded table shares one Extra between neighbouring
// levels that carry the same values, so a Definition stays small.
type Definition struct {
	ID    ID
	Level int

	Name       string
	Activation Activation
	Target     Target
	Element    Element
	RecallType RecallType

	Magic  bool
	Potion bool

	IgnoreResists bool
	StaticReuse   bool
	StaticHitTime bool
	IgnoreShield  bool

	Overhit          bool
	KillByDOT        bool
	SuicideAttack    bool
	SiegeSummonSkill bool

	// Advanced marks a SIEGE_FLAG skill putting up an advanced siege
	// headquarters, with twice the HP.
	Advanced bool

	// IsCubic selects the cubic branch of a SUMMON skill: Extra.NpcID is
	// then the cubic type id (matching cubic.ID) the skill grants.
	IsCubic bool

	NextActionIsAttack bool
	Debuff             bool
	Offensive          bool

	// HeroSkill reports whether holding hero status grants this skill,
	// independent of anything the level's own data says.
	HeroSkill bool

	DirectHPDamage   bool
	Dance            bool
	StayAfterDeath   bool
	CanBeReflected   bool
	CanBeDispelled   bool
	ClanSkill        bool
	SimultaneousCast bool

	Power float32

	MPConsume        int
	MPInitialConsume int
	HPConsume        int

	ItemConsumeCount int
	ItemConsumeID    int

	CastRange   int
	EffectRange int

	AbnormalLevel int

	HitTime  int
	CoolTime int

	ReuseDelay int

	Radius int

	MagicLevel  int
	LevelDepend int

	// SkillType is the raw effect-classification tag the skill carries. The
	// loader rejects an unknown one (see KnownSkillType); the skill handler
	// registry dispatches on it.
	SkillType string

	BaseCritRate int
	AggroPoints  int

	Flight *Flight

	Conditions []ConditionClause
	Funcs      []FuncTemplate
	Effects    []EffectTemplate

	// extra is shared, never changed once the Definition is built; nil
	// reads as the zero Extra.
	extra *Extra
}

// Extra holds the attributes of one skill level that only some skill types
// carry or read. A Definition shares it read-only with other levels; treat
// every value reached through it, slices included, as immutable.
type Extra struct {
	SharedReuse *Ref

	EquipDelay int

	TargetConsumeCount int
	TargetConsumeID    int

	EffectAbnormalLevel int
	NegateLevel         int

	Attribute         string
	NegateTypes       []string
	NegateIDs         []int
	MaxNegatedEffects int

	// Stat names the formula key a func/condition attached elsewhere reads
	// or writes; interpreting it is that engine's job, not this type's.
	Stat string

	// EffectType is the raw classification tag of the skill's optional,
	// separately classified side effect.
	EffectType string

	EffectID    int
	EffectPower int
	EffectLevel int

	// EffectNpcID is the template id of the world actor a signet-family
	// skill (SIGNET/SIGNET_CASTTIME) spawns to carry its periodic area
	// effect. -1 when unset.
	EffectNpcID int

	BaseLandRate int

	// NpcID is the cubic type id a SUMMON skill grants when the
	// Definition's IsCubic is set, and a servitor's spawn template id
	// otherwise.
	NpcID int

	// CubicActivationTime and CubicActivationChance are a granting SUMMON
	// skill's own "activationtime" (seconds between action ticks) and
	// "activationchance" (percent roll gating each non-Life-Cubic tick),
	// distinct from the unrelated ChanceSkillTrigger effect's
	// "activationChance" attribute captured by ActivationChance below.
	// SummonTotalLifeTime is the granted cubic's total lifetime in ms.
	CubicActivationTime   int
	CubicActivationChance int
	SummonTotalLifeTime   int
	// ExpPenalty is the share of kill exp a servitor summoned by this skill
	// withholds from its owner.
	ExpPenalty float32
	// SummonTimeLostIdle and SummonTimeLostActive are the lifetime, in ms,
	// a servitor summoned by this skill loses each second out of and in
	// combat. ItemConsumeIDOT and ItemConsumeCountOT are the upkeep item
	// its owner pays at each of ItemConsumeSteps checkpoints of that
	// lifetime.
	SummonTimeLostIdle   int
	SummonTimeLostActive int
	ItemConsumeIDOT      int
	ItemConsumeCountOT   int
	ItemConsumeSteps     int

	// WeaponsAllowed is the raw comma-separated weapon/armor type list a
	// level restricts casting to, or "" when unrestricted. Resolving a name
	// to its equipment mask is the item-type data's job, not this loader's.
	WeaponsAllowed string

	MinPledgeClass int

	TriggeredID      int
	TriggeredLevel   int
	ChanceType       string
	ActivationChance int

	MaxCharges int
	NumCharges int

	LethalChance1 int
	LethalChance2 int

	NextDanceCost int
	SoulShotBoost float32

	FlyRadius int
	FlyCourse float32

	Feed int

	// AbsorbPart and AbsorbAbs are a DRAIN skill's HP absorb: the caster
	// regains AbsorbAbs plus AbsorbPart of the drained HP per damaged target.
	AbsorbPart float32
	AbsorbAbs  int

	// ExtractableItems is the raw, unparsed product list a "capsule" skill
	// (one that unwraps into a random item) carries, or "" when the skill
	// isn't one. Structuring it into item/quantity/chance rows is deferred
	// until something consumes it.
	ExtractableItems string

	// CreateItemIDs, CreateItemCount and CreateItemRandomCount are what a
	// CREATE_ITEM skill gives its caster: one of CreateItemIDs, picked at
	// random, in CreateItemCount units plus 0 to CreateItemRandomCount-1
	// more, also picked at random. No ids or a zero count gives nothing.
	CreateItemIDs         []int32
	CreateItemCount       int
	CreateItemRandomCount int

	// AppearanceFace, AppearanceHairColor and AppearanceHairStyle are the
	// look a CHANGE_APPEARANCE skill gives every player it lands on; a
	// negative value leaves that part as it is.
	AppearanceFace      int
	AppearanceHairColor int
	AppearanceHairStyle int

	// DespawnDelay is how long, in milliseconds, the NPC a SPAWN skill
	// places (NpcID) stays in the world; 0 or less keeps it there.
	DespawnDelay int

	// TeleCoords is a RECALL or TELEPORT skill's fixed destination; when
	// set, it wins over the Definition's RecallType.
	TeleCoords *location.Location

	SelfEffects []EffectTemplate
}

// Extra returns a copy of d's type-specific attributes; the zero Extra when
// d carries none.
func (d Definition) Extra() Extra {
	if d.extra == nil {
		return Extra{}
	}
	return *d.extra
}

// WithExtra returns a copy of d carrying e as its type-specific attributes.
func (d Definition) WithExtra(e Extra) Definition {
	d.extra = &e
	return d
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
	Target     Target
	Element    Element
	RecallType RecallType

	Magic  bool
	Potion bool

	IgnoreResists bool
	StaticReuse   bool
	StaticHitTime bool
	IgnoreShield  bool

	Overhit          bool
	KillByDOT        bool
	SuicideAttack    bool
	SiegeSummonSkill bool
	Advanced         bool
	IsCubic          bool

	NextActionIsAttack bool
	Debuff             bool

	DirectHPDamage   bool
	Dance            bool
	StayAfterDeath   bool
	CanBeReflected   bool
	CanBeDispelled   bool
	ClanSkill        bool
	SimultaneousCast bool

	Power float32

	MPConsume        int
	MPInitialConsume int
	HPConsume        int

	ItemConsumeCount int
	ItemConsumeID    int

	CastRange   int
	EffectRange int

	AbnormalLevel int

	HitTime  int
	CoolTime int

	ReuseDelay int

	Radius int

	MagicLevel  int
	LevelDepend int

	SkillType string

	AggroPoints int

	Offensive    *bool
	BaseCritRate *int

	Flight *Flight

	Extra
}

// NewDefinition builds one level's Definition from id, level, name (the
// skill's own id and name, shared by every level) and a, that specific
// level's already-resolved attributes. It adds the fields a level's own
// data never carries: hero-skill membership, and the Offensive and
// BaseCritRate defaults derived from the level's other attributes when a
// leaves them unset.
func NewDefinition(id ID, level int, name string, a DefinitionAttrs) Definition {
	extra := a.Extra
	d := Definition{
		ID: id, Level: level, Name: name, HeroSkill: heroSkillIDs[id],

		Activation: a.Activation,
		Target:     a.Target,
		Element:    a.Element,
		RecallType: a.RecallType,

		Magic:  a.Magic,
		Potion: a.Potion,

		IgnoreResists: a.IgnoreResists,
		StaticReuse:   a.StaticReuse,
		StaticHitTime: a.StaticHitTime,
		IgnoreShield:  a.IgnoreShield,

		Overhit:          a.Overhit,
		KillByDOT:        a.KillByDOT,
		SuicideAttack:    a.SuicideAttack,
		SiegeSummonSkill: a.SiegeSummonSkill,
		Advanced:         a.Advanced,
		IsCubic:          a.IsCubic,

		NextActionIsAttack: a.NextActionIsAttack,
		Debuff:             a.Debuff,

		DirectHPDamage:   a.DirectHPDamage,
		Dance:            a.Dance,
		StayAfterDeath:   a.StayAfterDeath,
		CanBeReflected:   a.CanBeReflected,
		CanBeDispelled:   a.CanBeDispelled,
		ClanSkill:        a.ClanSkill,
		SimultaneousCast: a.SimultaneousCast,

		Power: a.Power,

		MPConsume:        a.MPConsume,
		MPInitialConsume: a.MPInitialConsume,
		HPConsume:        a.HPConsume,

		ItemConsumeCount: a.ItemConsumeCount,
		ItemConsumeID:    a.ItemConsumeID,

		CastRange:     a.CastRange,
		EffectRange:   a.EffectRange,
		AbnormalLevel: a.AbnormalLevel,

		HitTime:    a.HitTime,
		CoolTime:   a.CoolTime,
		ReuseDelay: a.ReuseDelay,

		Radius: a.Radius,

		MagicLevel:  a.MagicLevel,
		LevelDepend: a.LevelDepend,

		SkillType: a.SkillType,

		AggroPoints: a.AggroPoints,

		Flight: a.Flight,

		extra: &extra,
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

// IsDamage reports whether d is a skill type that deals damage.
func (d Definition) IsDamage() bool {
	switch d.SkillType {
	case "PDAM", "MDAM", "DRAIN", "BLOW", "CPDAMPERCENT", "DEATHLINK", "CHARGEDAM", "FATAL", "SIGNET_CASTTIME":
		return true
	}
	return false
}

// UsesSoulShot reports whether casting d spends a soulshot charge: the
// physical damage, blow, stun and charge-damage skill types.
func (d Definition) UsesSoulShot() bool {
	switch d.SkillType {
	case "BLOW", "PDAM", "STUN", "CHARGEDAM":
		return true
	}
	return false
}

// UsesSpiritShot reports whether casting d spends a spiritshot charge: every
// magic skill.
func (d Definition) UsesSpiritShot() bool {
	return d.Magic
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
// the shape a "sharedReuse" attribute carries. Exactly two dash-separated
// parts are required: a level is never negative, so a value like "100--1"
// is corrupt data rather than a reference to level -1.
func ParseRef(raw string) (Ref, error) {
	parts := strings.Split(raw, "-")
	if len(parts) != 2 {
		return Ref{}, fmt.Errorf("want \"skillId-level\"")
	}
	id, level := parts[0], parts[1]
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
