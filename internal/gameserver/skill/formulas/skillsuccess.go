package formulas

// meleeAttackRange is the cast range (in game units) below which a
// non-magic skill still counts as "melee" for reflect purposes.
const meleeAttackRange = 40

// SkillSuccessInput is the fully-resolved modifier set an effect-landing
// roll multiplies together. Every field is a number the caster/target pair
// has already computed (base effect power, the target's stat-based
// resistance, the caster's magic-attack ratio, the level-difference scale)
// — this function only clamps and combines them, matching the doc.go
// convention of taking already-resolved numbers rather than actors.
type SkillSuccessInput struct {
	BaseChance    float64
	StatModifier  float64
	VulnModifier  float64
	MAtkModifier  float64
	LevelModifier float64
	IgnoreResists bool
	// Shield is the target's already-resolved shield-block outcome against
	// this cast. A perfect block fails the roll unconditionally, taking
	// priority over IgnoreResists.
	Shield ShieldDefense
}

// SkillSuccessRate returns the percent chance that an effect-landing roll
// against in succeeds: the product of every modifier, clamped to [1, 99].
// A perfect shield block fails the roll outright, before anything else is
// considered. A skill that ignores resistances otherwise skips the
// modifiers and clamp entirely, returning BaseChance as-is.
func SkillSuccessRate(in SkillSuccessInput) float64 {
	if in.Shield == ShieldPerfect {
		return 0
	}
	if in.IgnoreResists {
		return in.BaseChance
	}
	rate := in.BaseChance * in.StatModifier * in.VulnModifier * in.MAtkModifier * in.LevelModifier
	if rate < 1 {
		return 1
	}
	if rate > 99 {
		return 99
	}
	return rate
}

// SkillSucceeds reports whether an effect-landing roll lands, given rate
// (from SkillSuccessRate) and roll, a uniform random draw in [0, 100).
func SkillSucceeds(rate float64, roll int) bool {
	return float64(roll) < rate
}

// SkillReflectInput is the fully-resolved state needed to decide whether a
// target reflects an incoming skill back at its caster.
type SkillReflectInput struct {
	IgnoreResists  bool
	CanBeReflected bool
	Magic          bool
	CastRange      int
	// ReflectChance is the target's already-resolved percent chance to
	// reflect this skill (REFLECT_SKILL_MAGIC or REFLECT_SKILL_PHYSIC).
	ReflectChance float64
}

// SkillReflects reports whether the target reflects the skill back at its
// caster, given roll, a uniform random draw in [0, 100). Skills that
// ignore resistances or can't be reflected never reflect; neither does a
// non-magic skill cast beyond melee range.
func SkillReflects(in SkillReflectInput, roll int) bool {
	if in.IgnoreResists || !in.CanBeReflected {
		return false
	}
	if !in.Magic && (in.CastRange == -1 || in.CastRange > meleeAttackRange) {
		return false
	}
	return float64(roll) < in.ReflectChance
}

// RevivePower returns the HP percentage a resurrection restores:
// skillPower scaled by the caster's already-resolved WIT bonus, capped 20
// points above the base skill power and never below it, then hard-capped
// at 90. 0 and 100 skill power pass through unchanged.
func RevivePower(witBonus, skillPower float64) float64 {
	if skillPower == 0 || skillPower == 100 {
		return skillPower
	}

	revivePower := skillPower * witBonus
	if revivePower-skillPower > 20 {
		revivePower = skillPower + 20
	}
	if revivePower < skillPower {
		revivePower = skillPower
	}
	if revivePower > 90 {
		revivePower = 90
	}
	return revivePower
}

// CancelSuccessRate returns the percent chance, clamped to [minRate,
// maxRate], that one cancel roll strips an active effect: effectPeriod is
// the effect's remaining duration in seconds (divided down like the
// reference integer arithmetic); diffLevel is the cancel skill's magic
// level minus the target's level; baseRate is the cancel skill's power;
// vuln is the target's already-resolved cancel-vulnerability multiplier.
func CancelSuccessRate(effectPeriod, diffLevel int, baseRate, vuln float64, minRate, maxRate int) float64 {
	rate := (float64(2*diffLevel) + baseRate + float64(effectPeriod/120)) * vuln
	if rate < float64(minRate) {
		return float64(minRate)
	}
	if rate > float64(maxRate) {
		return float64(maxRate)
	}
	return rate
}

// CancelSucceeds reports whether one cancel roll strips the effect, given
// rate (from CancelSuccessRate) and roll, a uniform random draw in
// [0, 100).
func CancelSucceeds(rate float64, roll int) bool {
	return float64(roll) < rate
}

// EffectCancelSuccessRate returns the percent chance, clamped to [25, 75],
// that a self-contained cancel effect (as opposed to the targeted
// cancel-family skill covered by CancelSuccessRate) strips one candidate
// effect. Unlike CancelSuccessRate, every term is combined as an integer
// and only the power term is scaled by vulnerability: casterMagicLevel is
// the cancel effect's own owning-skill magic level, candidateMagicLevel and
// candidatePeriod are the candidate effect's owning-skill magic level and
// remaining duration in seconds, and power*vuln truncates to an integer
// before it's added in.
func EffectCancelSuccessRate(casterMagicLevel, candidateMagicLevel, candidatePeriod int, power, vuln float64) int {
	rate := 2*(casterMagicLevel-candidateMagicLevel) + candidatePeriod/120 + int(power*vuln)
	if rate < 25 {
		return 25
	}
	if rate > 75 {
		return 75
	}
	return rate
}
