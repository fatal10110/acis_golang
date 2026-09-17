package skill

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// blessedSpiritshotCharged reports whether caster holds a blessed spiritshot
// charge; a cast with no caster never does.
func blessedSpiritshotCharged(caster Creature) bool {
	return caster != nil && caster.BlessedSpiritshotCharged()
}

// resolveShieldDefense returns def's shield-block outcome against target,
// or ShieldFailed when the skill ignores shields entirely or target is not a
// creature.
func resolveShieldDefense(caster Creature, target Actor, def modelskill.Definition) formulas.ShieldDefense {
	creature, ok := asCreature(target)
	if def.IgnoreShield || !ok {
		return formulas.ShieldFailed
	}
	return creature.ShieldDefense(caster, def, false)
}

type disablersHandler struct{}

// Types lists all 15 skill types the reference handler covers.
func (disablersHandler) Types() []string {
	return []string{
		"STUN", "ROOT", "SLEEP", "PARALYZE", "MUTE", "CONFUSION",
		"FAKE_DEATH", "BETRAY", "NEGATE", "CANCEL_DEBUFF",
		"AGGREDUCE", "AGGREDUCE_CHAR", "AGGREMOVE", "ERASE", "AGGDAMAGE",
	}
}

func (disablersHandler) Use(cast Cast) {
	skillType := skillTypeKey(cast.Skill.SkillType)

	for _, obj := range cast.Targets {
		target, ok := asCreature(obj)
		if !ok {
			continue
		}
		if target.Dead() || (target.Invul() && !target.Paralyzed()) {
			continue
		}
		if cast.Skill.Offensive && hasEffectType(target.EffectList(), "BLOCK_DEBUFF") {
			continue
		}

		switch skillType {
		case "BETRAY":
			disableWithSuccessCheck(cast, target)
		case "FAKE_DEATH":
			applyCastEffects(cast, target, cast.Skill, cast.Skill.Effects)
		case "ROOT", "STUN", "SLEEP", "PARALYZE":
			disableReflectable(cast, target)
		case "MUTE":
			disableMute(cast, target)
		case "CONFUSION":
			disableConfusion(cast, target)
		case "AGGREDUCE":
			disableAggReduce(cast, target)
		case "AGGREDUCE_CHAR":
			disableAggReduceChar(cast, target)
		case "AGGREMOVE":
			disableAggRemove(cast, target)
		case "ERASE":
			disableErase(cast, target)
		case "NEGATE":
			disableNegate(cast, target)
		case "CANCEL_DEBUFF":
			disableCancelDebuff(cast, target)
		case "AGGDAMAGE":
			disableAggDamage(cast, target)
		}
	}

	applySelfEffects(cast, cast.Skill)
}

// checkSkillSuccess rolls an effect-landing attempt of def against target,
// folding in the caster's blessed-spiritshot charge and the target's
// shield-block outcome against this cast. ok is false when target exposes
// no resolved-landing-rate source, letting a caller decide whether to treat
// that as "doesn't apply" or fall back.
func checkSkillSuccess(caster Creature, target Actor, def modelskill.Definition) (succeeded, ok bool) {
	return checkSkillSuccessBSS(caster, target, def, blessedSpiritshotCharged(caster))
}

// checkSkillSuccessBSS is checkSkillSuccess with the blessed-spiritshot
// input forced to bss rather than read from caster's real charge state —
// Blow.java hardcodes this input to true regardless of the caster's actual
// charge, unlike every other landing-rate roll in the reference.
func checkSkillSuccessBSS(caster Creature, target Actor, def modelskill.Definition, bss bool) (succeeded, ok bool) {
	return checkSkillSuccessBSSWithShield(caster, target, def, bss, resolveShieldDefense(caster, target, def))
}

func checkSkillSuccessBSSWithShield(caster Creature, target Actor, def modelskill.Definition, bss bool, shield formulas.ShieldDefense) (succeeded, ok bool) {
	src, ok := asCreature(target)
	if !ok {
		return false, false
	}
	in, ok := src.SkillSuccessInput(caster, def, bss, shield)
	if !ok {
		return false, false
	}
	rate := formulas.SkillSuccessRate(in)
	return formulas.SkillSucceeds(rate, rnd.Get(100)), true
}

// disableAggDamage applies an AGGDAMAGE skill's effects unconditionally (no
// landing roll, no reflect, matching Disablers.java's AGGDAMAGE case) and,
// for an attackable target that can also report its level, notifies its AI
// of the caster's aggression at power/(targetLevel+7)*150.
func disableAggDamage(cast Cast, target Creature) {
	if target.Attackable() {
		power := int(float64(cast.Skill.Power) / float64(target.Level()+7) * 150)
		target.NotifyAggression(cast.Caster, power)
	}
	applyCastEffects(cast, target, cast.Skill, cast.Skill.Effects)
}

func disableErase(cast Cast, target Creature) {
	succeeded, ok := checkSkillSuccess(cast.Caster, target, cast.Skill)
	if !ok || !succeeded {
		return
	}
	servitor, ok := target.(Summon)
	if !ok || servitor.SiegeSummon() {
		return
	}
	owner := servitor.SummonOwner()
	if owner == nil {
		return
	}
	servitor.UnSummon(owner)
	owner.ServitorVanished()
}

// reflectTarget returns the effect's actual destination: the original
// target, or the caster when the target reflects the skill back.
func reflectTarget(cast Cast, target Creature) Creature {
	in := target.SkillReflectInput(cast.Skill)
	in.SkillType = skillTypeKey(cast.Skill.SkillType)
	if !formulas.SkillReflects(in, rnd.Get(100)) {
		return target
	}
	return cast.Caster
}

func disableWithSuccessCheck(cast Cast, target Creature) {
	succeeded, ok := checkSkillSuccess(cast.Caster, target, cast.Skill)
	if !ok || !succeeded {
		return
	}
	applyCastEffects(cast, target, cast.Skill, cast.Skill.Effects)
}

// disableReflectable rolls the shield defense against the original target
// before the reflect swap, matching Disablers.java:64 (calcShldUse against
// targetCreature, once, ahead of the switch) and :80-83 (the reflect
// reassignment happens after sDef is already fixed, and calcSkillSuccess
// still consumes that pre-swap sDef even when the reflected cast now lands
// on the caster).
func disableReflectable(cast Cast, target Creature) {
	shield := resolveShieldDefense(cast.Caster, target, cast.Skill)
	effected := reflectTarget(cast, target)
	if effected == nil {
		return
	}
	succeeded, ok := checkSkillSuccessBSSWithShield(cast.Caster, effected, cast.Skill, blessedSpiritshotCharged(cast.Caster), shield)
	if !ok || !succeeded {
		return
	}
	applyCastEffects(cast, effected, cast.Skill, cast.Skill.Effects)
}

// disableMute rolls the shield defense against the original target before
// the reflect swap; see disableReflectable's comment (Disablers.java:64,
// :90-93 for the MUTE case specifically).
func disableMute(cast Cast, target Creature) {
	shield := resolveShieldDefense(cast.Caster, target, cast.Skill)
	effected := reflectTarget(cast, target)
	if effected == nil {
		return
	}
	succeeded, ok := checkSkillSuccessBSSWithShield(cast.Caster, effected, cast.Skill, blessedSpiritshotCharged(cast.Caster), shield)
	if !ok || !succeeded {
		return
	}
	stopSkillType(effected.EffectList(), skillTypeKey(cast.Skill.SkillType))
	applyCastEffects(cast, effected, cast.Skill, cast.Skill.Effects)
}

func disableConfusion(cast Cast, target Creature) {
	if !target.Attackable() {
		return
	}
	succeeded, ok := checkSkillSuccess(cast.Caster, target, cast.Skill)
	if !ok || !succeeded {
		return
	}
	stopSkillType(target.EffectList(), skillTypeKey(cast.Skill.SkillType))
	applyCastEffects(cast, target, cast.Skill, cast.Skill.Effects)
}

// disableAggReduce applies the skill's effects and, for a positive skill
// power, subtracts it from every hate entry the target's threat table
// holds. The reference handler also covers a zero-or-negative power that
// instead subtracts a generic AGGRESSION stat delta; that needs a stat
// resolution this port has no generic model for yet, so it's skipped.
func disableAggReduce(cast Cast, target Creature) {
	// Only an NPC holds the aggro tables this skill reduces.
	if !target.Attackable() {
		return
	}
	applyCastEffects(cast, target, cast.Skill, cast.Skill.Effects)
	if cast.Skill.Power > 0 {
		target.ReduceAllAggroHate(float64(cast.Skill.Power))
	}
}

func disableAggReduceChar(cast Cast, target Creature) {
	succeeded, ok := checkSkillSuccess(cast.Caster, target, cast.Skill)
	if !ok || !succeeded {
		return
	}
	if cast.Caster != nil {
		target.StopAggroHate(cast.Caster)
		target.StopHateList(cast.Caster)
	}
	applyCastEffects(cast, target, cast.Skill, cast.Skill.Effects)
}

func disableAggRemove(cast Cast, target Creature) {
	if !target.Attackable() || target.RaidRelated() {
		return
	}
	succeeded, ok := checkSkillSuccess(cast.Caster, target, cast.Skill)
	if !ok || !succeeded {
		return
	}
	if cast.Skill.Target == modelskill.TargetUndead {
		if !target.Undead() {
			return
		}
	}
	target.ClearAggroTables()
}

// disableNegate strips effects matching the skill's negate configuration,
// then applies the skill's own effects. Explicit negate-by-id lists and an
// unconditional (NegateLevel == -1) negate-by-type list are ported; a
// level-gated negate-by-type needs each active effect's abnormal level,
// which isn't tracked on a live effect yet, so it's skipped.
func disableNegate(cast Cast, target Creature) {
	effected := reflectTarget(cast, target)
	if effected == nil {
		return
	}
	list := effected.EffectList()

	if len(cast.Skill.NegateIDs) > 0 {
		for _, id := range cast.Skill.NegateIDs {
			if id == 0 {
				continue
			}
			removeMatching(list, 0, func(e *effect.Effect) bool {
				return int(e.Skill.ID) == id
			})
		}
	} else if cast.Skill.NegateLevel == -1 {
		for _, negateType := range cast.Skill.NegateTypes {
			removeMatching(list, 0, func(e *effect.Effect) bool {
				return e.Template.StackOrder != 99 &&
					(strings.EqualFold(e.Skill.SkillType, negateType) || strings.EqualFold(e.Template.EffectType, negateType))
			})
		}
	}

	applyCastEffects(cast, effected, cast.Skill, cast.Skill.Effects)
}

func disableCancelDebuff(cast Cast, target Creature) {
	removeMatching(target.EffectList(), cast.Skill.MaxNegatedEffects, func(e *effect.Effect) bool {
		return e.Skill.Debuff && e.Skill.CanBeDispelled && e.Template.StackOrder != 99
	})
}

func stopSkillType(list *effect.List, skillType string) {
	removeMatching(list, 0, func(e *effect.Effect) bool {
		return e.Template.StackOrder != 99 && strings.EqualFold(e.Skill.SkillType, skillType)
	})
}

func hasEffectType(list *effect.List, tag string) bool {
	if list == nil {
		return false
	}
	for _, e := range list.All() {
		if strings.EqualFold(e.ClassTag(), tag) {
			return true
		}
	}
	return false
}
