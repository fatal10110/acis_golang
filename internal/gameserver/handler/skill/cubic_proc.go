package skill

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// UseCubic applies a cubic's fired skill, cast with the cubic's owner as
// Caster, to its single target. mAtk is the cubic's own M.Atk, fixed when
// it was granted. Disabler, magic-damage, continuous and drain skills take
// the cubic branches below: they land through the cubic's landing roll,
// strike with the cubic damage formula, and never spend the owner's
// spiritshot. Any other skill type goes through its registered handler as
// the owner's own cast would.
func (r *Registry) UseCubic(cast Cast, mAtk float64) (Result, bool) {
	if r == nil {
		return Result{}, false
	}
	switch skillTypeKey(cast.Skill.SkillType) {
	case "PARALYZE", "STUN", "ROOT", "AGGDAMAGE", "MDAM", "POISON", "DEBUFF", "DOT", "DRAIN":
	default:
		return r.UseResult(cast)
	}

	messages := &messageLog{sink: cast.Sink}
	cast.messages = messages
	result := Result{messages: messages}
	cast.resisted = &result
	if cast.Caster != nil {
		for _, obj := range cast.Targets {
			if target, ok := asCreature(obj); ok {
				r.useCubicOn(cast, target, mAtk, &result)
			}
		}
	}
	result.Messages = messages.pending
	return result, true
}

func (r *Registry) useCubicOn(cast Cast, target Creature, mAtk float64, result *Result) {
	switch skillTypeKey(cast.Skill.SkillType) {
	case "PARALYZE", "STUN", "ROOT", "AGGDAMAGE":
		cubicDisable(cast, target, mAtk)
	case "MDAM":
		cubicMdam(cast, target, mAtk, r.magicFailures, result)
	case "POISON", "DEBUFF", "DOT":
		cubicContinuous(cast, target, mAtk, result)
	case "DRAIN":
		cubicDrain(cast, target, r.magicFailures, result)
	}
}

// cubicLands is a cubic's own landing roll for its skill on target: a
// reflected skill or a perfect shield block fails it outright; otherwise it
// rolls the cubic landing rate.
func cubicLands(cast Cast, target Creature, mAtk float64, bss bool, shield formulas.ShieldDefense) bool {
	if skillReflected(cast, target) {
		return false
	}
	in, ok := creature.ResolveCubicSkillSuccessInput(cast.Caster, target, cast.Skill, mAtk, bss, shield)
	if !ok {
		return false
	}
	return formulas.SkillSucceeds(formulas.SkillSuccessRate(in), rnd.Get(100))
}

// landCubicEffects lands the skill's effect templates on target with the
// owner as effector, with no shield or blessed-spiritshot outcome carried
// into the per-template rolls.
func landCubicEffects(cast Cast, target Creature) {
	applyCastEffects(cast, target, cast.Skill, cast.Skill.Effects, formulas.ShieldFailed, false)
}

// cubicDisable is a cubic's PARALYZE, STUN, ROOT or AGGDAMAGE proc. A
// failed landing roll is silent. AGGDAMAGE then provokes an NPC target by
// 150 * power / (level + 7).
func cubicDisable(cast Cast, target Creature, mAtk float64) {
	if target.Dead() {
		return
	}
	bss := blessedSpiritshotCharged(cast.Caster)
	shield := resolveShieldDefense(cast.Caster, target, cast.Skill)
	if !cubicLands(cast, target, mAtk, bss, shield) {
		return
	}
	if skillTypeKey(cast.Skill.SkillType) == "AGGDAMAGE" && target.Attackable() {
		target.NotifyAggression(cast.Caster, int(150*float64(cast.Skill.Power)/float64(target.Level()+7)))
	}
	landCubicEffects(cast, target)
}

// cubicContinuous is a cubic's POISON, DEBUFF or DOT proc. An offensive
// skill that fails its landing roll tells the owner the attack failed.
func cubicContinuous(cast Cast, target Creature, mAtk float64, result *Result) {
	if target.Dead() {
		return
	}
	if cast.Skill.Offensive {
		shield := resolveShieldDefense(cast.Caster, target, cast.Skill)
		if !cubicLands(cast, target, mAtk, blessedSpiritshotCharged(cast.Caster), shield) {
			result.AttackFailed++
			result.record(AttackFailedMessage{})
			return
		}
	}
	landCubicEffects(cast, target)
}

// cubicStrike rolls the owner's magic critical and the target's shield,
// then the cubic strike damage, reporting any magic failure first.
func cubicStrike(cast Cast, target Creature, magicFailures bool, result *Result) (damage int, crit bool, shield formulas.ShieldDefense) {
	crit = formulas.MCritSucceeds(int(cast.Caster.MagicCriticalRate()), cast.Caster.Roll(1000))
	shield = resolveShieldDefense(cast.Caster, target, cast.Skill)
	in, ok := creature.ResolveCubicMagicDamageInput(cast.Caster, target, cast.Skill, crit, shield, magicFailures)
	if !ok {
		return 0, crit, shield
	}
	reportMagicFailure(cast, target, in.Failure, result)
	return javaInt(formulas.CubicMagicDamage(in)), crit, shield
}

// cubicMdam is a cubic's MDAM proc. A reflected strike deals nothing. A hit
// rolls the target's cast break and reports the damage; a skill with
// effects then replaces its own effects on the target when the cubic
// landing roll passes (silently otherwise), and the target takes the HP
// last.
func cubicMdam(cast Cast, target Creature, mAtk float64, magicFailures bool, result *Result) {
	if target.Dead() {
		return
	}
	damage, crit, shield := cubicStrike(cast, target, magicFailures, result)
	if skillReflected(cast, target) {
		damage = 0
	}
	if damage <= 0 {
		return
	}
	breaker, breakFirst := target.(earlyCastBreaker)
	if breakFirst {
		breaker.BreakCastOnDamage(float64(damage))
	}
	recordDamage(result, cast.Caster, target, damage, crit, false)
	if len(cast.Skill.Effects) > 0 {
		stopEffectsBySkillID(target.EffectList(), cast.Skill.ID)
		if cubicLands(cast, target, mAtk, blessedSpiritshotCharged(cast.Caster), shield) {
			landCubicEffects(cast, target)
		}
	}
	if breakFirst {
		breaker.ReduceHPWithoutCastBreak(float64(damage), cast.Caster, cast.Skill)
	} else {
		target.ReduceHP(float64(damage), cast.Caster, cast.Skill)
	}
}

// cubicDrain is a cubic's DRAIN proc. The owner regains absorbAbs plus
// absorbPart of the full damage, with no CP netting or HP cap, and no
// invulnerability skip. The target then takes the HP, rolls its cast break
// on it, and the damage is reported; a corpse drain only feeds the owner.
func cubicDrain(cast Cast, target Creature, magicFailures bool, result *Result) {
	corpseMob := cast.Skill.Target == modelskill.TargetCorpseMob
	if target.AlikeDead() && !corpseMob {
		return
	}
	damage, crit, _ := cubicStrike(cast, target, magicFailures, result)
	if damage <= 0 {
		return
	}
	// In single precision; the explicit conversion rounds the product
	// before the sum, so the two steps are never fused into one.
	part := float32(cast.Skill.AbsorbPart * float32(damage))
	if cast.Caster.AddHP(float64(float32(cast.Skill.AbsorbAbs)+part)) > 0 && cast.Caster.Kind() == actor.KindPlayer {
		result.record(CasterVitalsChanged{})
	}
	if target.Dead() && corpseMob {
		return
	}
	if breaker, ok := target.(earlyCastBreaker); ok {
		breaker.ReduceHPWithoutCastBreak(float64(damage), cast.Caster, cast.Skill)
		breaker.BreakCastOnDamage(float64(damage))
	} else {
		target.ReduceHP(float64(damage), cast.Caster, cast.Skill)
	}
	recordDamage(result, cast.Caster, target, damage, crit, false)
}
