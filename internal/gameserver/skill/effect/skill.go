package effect

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

type Type string

const (
	// TypeBuff is a beneficial persistent effect.
	TypeBuff Type = "BUFF"
	// TypeDebuff is a harmful persistent effect.
	TypeDebuff Type = "DEBUFF"
	// TypeDamOverTime is a periodic HP damage effect.
	TypeDamOverTime Type = "DMG_OVER_TIME"
	// TypeFear is a forced flee disabler.
	TypeFear Type = "FEAR"
	// TypeRoot is a movement disabler.
	TypeRoot Type = "ROOT"
	// TypeSleep is an action disabler.
	TypeSleep Type = "SLEEP"
	// TypeStun is an attack, cast, and movement disabler.
	TypeStun Type = "STUN"
)

// Skill carries the skill fields the effect container needs for ordering and
// duplicate handling, plus the classification tags a disabling/dispelling
// skill needs to recognize an already-active effect as "the same kind" of
// disable or "eligible to be stripped".
type Skill struct {
	ID modelskill.ID
	// Level is the applied level of the skill that owns this effect
	// instance's template.
	Level int
	// SkillType is the raw datapack skill-type tag (e.g. "BUFF", "REFLECT").
	// It drives the buff-slot family used by the list's cap enforcement.
	SkillType      string
	Debuff         bool
	Toggle         bool
	KillByDOT      bool
	CanBeDispelled bool
	// Dance marks a song/dance skill, consulted by a signet-family effect's
	// area tick that cancels dances on nearby targets.
	Dance bool

	// MagicLevel is the owning skill's casting level, read by cancel-family
	// effects to compare a caster's cancel power against each candidate
	// effect's own owning-skill level.
	MagicLevel int
	// LevelDepend is the owning skill's configured level-dependency bonus,
	// read by a magic-resist roll (e.g. a caster-applied spoil effect).
	LevelDepend int
	// AbnormalLevel and EffectAbnormalLevel are the owning skill's cancel-
	// threshold tags: EffectAbnormalLevel applies when EffectType is set,
	// AbnormalLevel otherwise. A negate-family effect strips a candidate
	// only when the candidate's applicable level is within its threshold.
	AbnormalLevel       int
	EffectAbnormalLevel int
	// EffectType is the owning skill's classification tag (distinct from
	// the per-effect-template tag exposed by Effect.ClassTag), consulted
	// alongside SkillType when a negate-family effect matches a candidate
	// by classification.
	EffectType string

	// MaxNegatedEffects caps how many candidates a cancel-family effect
	// strips in one activation; 0 means unlimited.
	MaxNegatedEffects int
	// NegateLevel, NegateIDs, and NegateTypes configure a negate-family
	// effect: NegateIDs strips candidates by their owning skill id,
	// NegateTypes strips candidates by classification (gated by
	// NegateLevel, or ungated when -1).
	NegateLevel int
	NegateIDs   []int
	NegateTypes []string

	// FlyRadius is the owning skill's configured forced-flight distance,
	// read by the knockback effect kind to size its landing offset.
	FlyRadius int
}

func (s Skill) sevenSigns() bool {
	return s.ID > 4360 && s.ID < 4367
}

// buffSlotFamily is the set of skill types that occupy (and can be evicted
// from) an owner's limited buff slots.
var buffSlotFamily = map[string]bool{
	"BUFF":             true,
	"REFLECT":          true,
	"HEAL_PERCENT":     true,
	"HEAL_STATIC":      true,
	"MANAHEAL_PERCENT": true,
	"COMBATPOINTHEAL":  true,
}

func (s Skill) buffSlot() bool {
	return buffSlotFamily[s.SkillType]
}

// Effect is one live skill effect managed by a List. Hook fields are optional;
// absent hooks behave as a successful no-op.
