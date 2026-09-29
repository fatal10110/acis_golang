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
	// Sampled before any target: this reading decides which spiritshot the
	// cast spends at the end.
	bsps := blessedSpiritshotCharged(cast.Caster)

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

		// The shield block is rolled once per target, against the original
		// target and ahead of any reflect swap, whichever type follows; the
		// skill's own landing roll and its per-template landings reuse it.
		land := landing{shield: resolveShieldDefense(cast.Caster, target, cast.Skill), bss: bsps}

		switch skillType {
		case "BETRAY":
			disableWithSuccessCheck(cast, target, land)
		case "FAKE_DEATH":
			land.apply(cast, target)
		case "ROOT", "STUN", "SLEEP", "PARALYZE":
			disableReflectable(cast, target, land)
		case "MUTE":
			disableMute(cast, target, land)
		case "CONFUSION":
			disableConfusion(cast, target, land)
		case "AGGREDUCE":
			disableAggReduce(cast, target, land)
		case "AGGREDUCE_CHAR":
			disableAggReduceChar(cast, target, land)
		case "AGGREMOVE":
			disableAggRemove(cast, target, land)
		case "ERASE":
			disableErase(cast, target, land)
		case "NEGATE":
			disableNegate(cast, target, land)
		case "CANCEL_DEBUFF":
			disableCancelDebuff(cast, target)
		case "AGGDAMAGE":
			disableAggDamage(cast, target, land)
		}
	}

	applySelfEffects(cast, cast.Skill)
	// A cubic proc leaves its owner's spiritshot charged.
	if !cast.Cubic {
		writeSpiritshot(cast.Caster, bsps, cast.Skill.StaticReuse)
	}
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

// landing is one Disablers target's resolved shield outcome and the cast's
// blessed-spiritshot sample, shared by the skill's landing roll and every
// per-template effect landing on that target.
type landing struct {
	shield formulas.ShieldDefense
	bss    bool
}

// succeeds rolls cast's skill landing against effected with the resolved
// inputs.
func (l landing) succeeds(cast Cast, effected Creature) (succeeded, ok bool) {
	return checkSkillSuccessBSSWithShield(cast.Caster, effected, cast.Skill, l.bss, l.shield)
}

// apply lands cast's effect templates on effected with the resolved inputs.
func (l landing) apply(cast Cast, effected Creature) {
	applyCastEffects(cast, effected, cast.Skill, cast.Skill.Effects, l.shield, l.bss)
}

// disableAggDamage applies an AGGDAMAGE skill's effects unconditionally (no
// landing roll, no reflect, matching Disablers.java's AGGDAMAGE case) and,
// for an attackable target that can also report its level, notifies its AI
// of the caster's aggression at power/(targetLevel+7)*150.
func disableAggDamage(cast Cast, target Creature, land landing) {
	if target.Attackable() {
		power := int(float64(cast.Skill.Power) / float64(target.Level()+7) * 150)
		target.NotifyAggression(cast.Caster, power)
	}
	land.apply(cast, target)
}

func disableErase(cast Cast, target Creature, land landing) {
	succeeded, ok := land.succeeds(cast, target)
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

func disableWithSuccessCheck(cast Cast, target Creature, land landing) {
	succeeded, ok := land.succeeds(cast, target)
	if !ok || !succeeded {
		return
	}
	land.apply(cast, target)
}

// disableReflectable keeps the shield outcome rolled against the original
// target even when the reflect swap turns the cast back on the caster: both
// the landing roll and the effects use that pre-swap outcome.
func disableReflectable(cast Cast, target Creature, land landing) {
	effected := reflectTarget(cast, target)
	if effected == nil {
		return
	}
	succeeded, ok := land.succeeds(cast, effected)
	if !ok || !succeeded {
		if ok {
			if _, player := asPlayer(cast.Caster); player {
				appendResisted(cast.resisted, effected, cast.Skill, 1, false)
			}
		}
		return
	}
	land.apply(cast, effected)
}

// disableMute keeps the pre-swap shield outcome the way disableReflectable
// does.
func disableMute(cast Cast, target Creature, land landing) {
	effected := reflectTarget(cast, target)
	if effected == nil {
		return
	}
	succeeded, ok := land.succeeds(cast, effected)
	if !ok || !succeeded {
		if ok {
			if _, player := asPlayer(cast.Caster); player {
				appendResisted(cast.resisted, effected, cast.Skill, 1, false)
			}
		}
		return
	}
	stopSkillType(effected.EffectList(), skillTypeKey(cast.Skill.SkillType))
	land.apply(cast, effected)
}

func disableConfusion(cast Cast, target Creature, land landing) {
	if !target.Attackable() {
		return
	}
	succeeded, ok := land.succeeds(cast, target)
	if !ok || !succeeded {
		return
	}
	stopSkillType(target.EffectList(), skillTypeKey(cast.Skill.SkillType))
	land.apply(cast, target)
}

// disableAggReduce applies the skill's effects and, for a positive skill
// power, subtracts it from every hate entry the target's threat table
// holds. The reference handler also covers a zero-or-negative power that
// instead subtracts a generic AGGRESSION stat delta; that needs a stat
// resolution this port has no generic model for yet, so it's skipped.
func disableAggReduce(cast Cast, target Creature, land landing) {
	// Only an NPC holds the aggro tables this skill reduces.
	if !target.Attackable() {
		return
	}
	land.apply(cast, target)
	if cast.Skill.Power > 0 {
		target.ReduceAllAggroHate(float64(cast.Skill.Power))
	}
}

func disableAggReduceChar(cast Cast, target Creature, land landing) {
	succeeded, ok := land.succeeds(cast, target)
	if !ok || !succeeded {
		return
	}
	if cast.Caster != nil {
		target.StopAggroHate(cast.Caster)
		target.StopHateList(cast.Caster)
	}
	land.apply(cast, target)
}

func disableAggRemove(cast Cast, target Creature, land landing) {
	if !target.Attackable() || target.RaidRelated() {
		return
	}
	succeeded, ok := land.succeeds(cast, target)
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
func disableNegate(cast Cast, target Creature, land landing) {
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

	land.apply(cast, effected)
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
