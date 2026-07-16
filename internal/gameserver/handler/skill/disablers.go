package skill

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

type disablerTarget interface {
	effectListTarget
	Dead() bool
	Invul() bool
	Paralyzed() bool
}

// skillSuccessSource supplies the already-resolved modifier set an
// effect-landing roll needs; ok is false when the target can't be rolled
// against at all.
type skillSuccessSource interface {
	SkillSuccessInput(caster any, def modelskill.Definition) (in formulas.SkillSuccessInput, ok bool)
}

// skillReflectSource optionally supplies the already-resolved state needed
// to decide whether the target reflects the skill; a target without one
// never reflects.
type skillReflectSource interface {
	SkillReflectInput(def modelskill.Definition) formulas.SkillReflectInput
}

// attackableMarker reports whether a target is an NPC-like combat entity,
// gating the skill types that only affect monsters, not players.
type attackableMarker interface {
	Attackable() bool
}

// aggroTables exposes an attackable target's two hate tables.
type aggroTables interface {
	AggroList() *attackable.ThreatTable
	HateList() *attackable.HateTable
}

// raidRelatedTarget optionally reports whether the target is a raid boss
// or minion, exempt from aggro-clearing skills; a target without one is
// treated as not raid-related.
type raidRelatedTarget interface {
	RaidRelated() bool
}

// undeadTarget optionally reports whether the target is undead; a target
// without one is treated as not undead.
type undeadTarget interface {
	Undead() bool
}

type disablersHandler struct{}

// Types lists 14 of the 15 skill types the reference handler covers.
// AGGDAMAGE needs an AI aggression-event notification pipeline that isn't
// wired yet, so it is left unregistered rather than half-ported into a no-op.
func (disablersHandler) Types() []string {
	return []string{
		"STUN", "ROOT", "SLEEP", "PARALYZE", "MUTE", "CONFUSION",
		"FAKE_DEATH", "BETRAY", "NEGATE", "CANCEL_DEBUFF",
		"AGGREDUCE", "AGGREDUCE_CHAR", "AGGREMOVE", "ERASE",
	}
}

func (disablersHandler) Use(cast Cast) {
	skillType := skillTypeKey(cast.Skill.SkillType)

	for _, obj := range cast.Targets {
		target, ok := obj.(disablerTarget)
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
			applyEffects(cast.Caster, target, cast.Skill, cast.Skill.Effects)
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
		}
	}

	applySelfEffects(cast.Caster, cast.Skill)
}

// checkSkillSuccess rolls an effect-landing attempt of def against target.
// ok is false when target exposes no resolved-landing-rate source, letting a
// caller decide whether to treat that as "doesn't apply" or fall back.
func checkSkillSuccess(caster any, target any, def modelskill.Definition) (succeeded, ok bool) {
	src, ok := target.(skillSuccessSource)
	if !ok {
		return false, false
	}
	in, ok := src.SkillSuccessInput(caster, def)
	if !ok {
		return false, false
	}
	rate := formulas.SkillSuccessRate(in)
	return formulas.SkillSucceeds(rate, rnd.Get(100)), true
}

type erasableSummon interface {
	SummonOwner() any
	SiegeSummon() bool
	UnSummon(owner any)
}

type servitorVanishNotifier interface {
	ServitorVanished()
}

func disableErase(cast Cast, target disablerTarget) {
	succeeded, ok := checkSkillSuccess(cast.Caster, target, cast.Skill)
	if !ok || !succeeded {
		return
	}
	summon, ok := target.(erasableSummon)
	if !ok || summon.SiegeSummon() {
		return
	}
	owner := summon.SummonOwner()
	if owner == nil {
		return
	}
	summon.UnSummon(owner)
	if notifier, ok := owner.(servitorVanishNotifier); ok {
		notifier.ServitorVanished()
	}
}

// reflectTarget returns the effect's actual destination: the original
// target, or the caster when the target reflects the skill back. It
// returns nil when the skill reflects but the caster isn't itself a valid
// disabler target (a duck-typing gap safer to drop than to guess through).
func reflectTarget(cast Cast, target disablerTarget) disablerTarget {
	src, ok := target.(skillReflectSource)
	if !ok {
		return target
	}
	if !formulas.SkillReflects(src.SkillReflectInput(cast.Skill), rnd.Get(100)) {
		return target
	}
	self, ok := cast.Caster.(disablerTarget)
	if !ok {
		return nil
	}
	return self
}

func disableWithSuccessCheck(cast Cast, target disablerTarget) {
	succeeded, ok := checkSkillSuccess(cast.Caster, target, cast.Skill)
	if !ok || !succeeded {
		return
	}
	applyEffects(cast.Caster, target, cast.Skill, cast.Skill.Effects)
}

func disableReflectable(cast Cast, target disablerTarget) {
	effected := reflectTarget(cast, target)
	if effected == nil {
		return
	}
	succeeded, ok := checkSkillSuccess(cast.Caster, effected, cast.Skill)
	if !ok || !succeeded {
		return
	}
	applyEffects(cast.Caster, effected, cast.Skill, cast.Skill.Effects)
}

func disableMute(cast Cast, target disablerTarget) {
	effected := reflectTarget(cast, target)
	if effected == nil {
		return
	}
	succeeded, ok := checkSkillSuccess(cast.Caster, effected, cast.Skill)
	if !ok || !succeeded {
		return
	}
	stopSkillType(effected.EffectList(), skillTypeKey(cast.Skill.SkillType))
	applyEffects(cast.Caster, effected, cast.Skill, cast.Skill.Effects)
}

func disableConfusion(cast Cast, target disablerTarget) {
	am, ok := target.(attackableMarker)
	if !ok || !am.Attackable() {
		return
	}
	succeeded, ok := checkSkillSuccess(cast.Caster, target, cast.Skill)
	if !ok || !succeeded {
		return
	}
	stopSkillType(target.EffectList(), skillTypeKey(cast.Skill.SkillType))
	applyEffects(cast.Caster, target, cast.Skill, cast.Skill.Effects)
}

// disableAggReduce applies the skill's effects and, for a positive skill
// power, subtracts it from every hate entry the target's threat table
// holds. The reference handler also covers a zero-or-negative power that
// instead subtracts a generic AGGRESSION stat delta; that needs a stat
// resolution this port has no generic model for yet, so it's skipped.
func disableAggReduce(cast Cast, target disablerTarget) {
	at, ok := target.(aggroTables)
	if !ok {
		return
	}
	applyEffects(cast.Caster, target, cast.Skill, cast.Skill.Effects)
	if cast.Skill.Power > 0 {
		at.AggroList().ReduceAllHate(float64(cast.Skill.Power))
	}
}

func disableAggReduceChar(cast Cast, target disablerTarget) {
	succeeded, ok := checkSkillSuccess(cast.Caster, target, cast.Skill)
	if !ok || !succeeded {
		return
	}
	if at, ok := target.(aggroTables); ok {
		if attacker, ok := cast.Caster.(attackable.Combatant); ok {
			at.AggroList().StopHate(attacker)
			at.HateList().StopHate(attacker)
		}
	}
	applyEffects(cast.Caster, target, cast.Skill, cast.Skill.Effects)
}

func disableAggRemove(cast Cast, target disablerTarget) {
	am, ok := target.(attackableMarker)
	if !ok || !am.Attackable() {
		return
	}
	if rr, ok := target.(raidRelatedTarget); ok && rr.RaidRelated() {
		return
	}
	succeeded, ok := checkSkillSuccess(cast.Caster, target, cast.Skill)
	if !ok || !succeeded {
		return
	}
	if cast.Skill.Target == modelskill.TargetUndead {
		ut, ok := target.(undeadTarget)
		if !ok || !ut.Undead() {
			return
		}
	}
	if at, ok := target.(aggroTables); ok {
		at.AggroList().Clear()
		at.HateList().Clear()
	}
}

// disableNegate strips effects matching the skill's negate configuration,
// then applies the skill's own effects. Explicit negate-by-id lists and an
// unconditional (NegateLevel == -1) negate-by-type list are ported; a
// level-gated negate-by-type needs each active effect's abnormal level,
// which isn't tracked on a live effect yet, so it's skipped.
func disableNegate(cast Cast, target disablerTarget) {
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

	applyEffects(cast.Caster, effected, cast.Skill, cast.Skill.Effects)
}

func disableCancelDebuff(cast Cast, target disablerTarget) {
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
