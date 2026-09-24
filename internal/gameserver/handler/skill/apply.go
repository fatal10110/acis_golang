package skill

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// applyEffects instantiates each of templates and adds it to effected's
// effect list, attributed to effector. def carries the owning skill's
// identity and stacking classification. A template naming an effect core
// this port hasn't wired yet (see effect.New) is skipped rather than
// failing the whole batch, matching how partially-modeled skill data
// degrades elsewhere in this package.
func applyEffects(effector effect.Actor, effected Actor, def modelskill.Definition, templates []modelskill.EffectTemplate) {
	applyEffectsWithLanding(effector, effected, def, templates, formulas.ShieldFailed, false)
}

func applyCastEffects(cast Cast, effected Actor, def modelskill.Definition, templates []modelskill.EffectTemplate) {
	cast.reportResisted(effected, def, applyEffectsWithLanding(cast.Caster, effected, def, templates, formulas.ShieldFailed, false))
}

// applyCasterSelfEffects lands a skill's self effects on its caster. Unlike
// every other landing it has no dead-effected refusal, so a caster the cast
// just killed still takes them.
func applyCasterSelfEffects(cast Cast, def modelskill.Definition) {
	cast.reportResisted(cast.Caster, def, landEffects(cast.Caster, cast.Caster, def, def.SelfEffects, formulas.ShieldFailed, false))
}

// applyEffectsWithLanding lands templates on effected, refusing a dead
// effected outright whichever handler resolved it.
func applyEffectsWithLanding(effector effect.Actor, effected Actor, def modelskill.Definition, templates []modelskill.EffectTemplate, shield formulas.ShieldDefense, bss bool) (resisted int) {
	if c, ok := asCreature(effected); ok && c.Dead() {
		return 0
	}
	return landEffects(effector, effected, def, templates, shield, bss)
}

func landEffects(effector effect.Actor, effected Actor, def modelskill.Definition, templates []modelskill.EffectTemplate, shield formulas.ShieldDefense, bss bool) (resisted int) {
	if len(templates) == 0 {
		return 0
	}
	// Only an effect participant owns an effect list to land on.
	target, ok := effected.(effect.Actor)
	if !ok {
		return 0
	}
	if !withinEffectRange(effector, target, def.EffectRange) {
		return 0
	}
	if offensiveEffectApplyBlocked(effector, target, def) {
		return 0
	}
	list := target.EffectList()
	if list == nil {
		return 0
	}

	meta := effect.SkillFromDefinition(def)
	if shield == formulas.ShieldPerfect {
		return 0
	}
	source, canRoll := asCreature(effected)
	for _, tmpl := range templates {
		if tmpl.EffectPowerSet && tmpl.EffectPower >= 0 {
			if !canRoll {
				if tmpl.Icon {
					resisted++
				}
				continue
			}
			in, ok := source.EffectSuccessInput(formulaCasterOf(effector), def, tmpl, bss, shield)
			if !ok || !formulas.SkillSucceeds(formulas.SkillSuccessRate(in), rnd.Get(100)) {
				if tmpl.Icon {
					resisted++
				}
				continue
			}
		}
		e, err := effect.New(meta, tmpl)
		if err != nil {
			continue
		}
		owner := list
		if e.SelfTarget {
			if effector == nil || effector.EffectList() == nil {
				continue
			}
			owner = effector.EffectList()
		}
		e.Effector = effector
		e.Effected = target
		owner.Add(e)
	}
	return resisted
}

// withinEffectRange mirrors L2Skill.getEffects' landing-time 3D radius
// check. Actors without a modeled position stay permissive like other
// optional handler capabilities.
func withinEffectRange(effector, effected effect.Actor, effectRange int) bool {
	if effectRange <= 0 || effector == nil || effector.ObjectID() == effected.ObjectID() {
		return true
	}
	ax, ay, az := effector.Position()
	bx, by, bz := effected.Position()
	return location.Location{X: ax, Y: ay, Z: az}.Distance3D(location.Location{X: bx, Y: by, Z: bz}) < float64(effectRange)
}

// offensiveEffectApplyBlocked is the landing-time refuse for offensive and
// debuff skills aimed at someone else: target invulnerability, or a caster
// not permitted to deal damage. Self-applies skip both.
func offensiveEffectApplyBlocked(effector, effected effect.Actor, def modelskill.Definition) bool {
	if !def.Offensive && !def.Debuff {
		return false
	}
	if effector != nil && effected != nil && effector.ObjectID() == effected.ObjectID() {
		return false
	}
	if c, ok := effected.(Creature); ok && c.Invul() {
		return true
	}
	// Every combatant is consulted, formula caster or not; only a
	// non-combatant effector is treated as sourceless and never blocked.
	cb, _ := effector.(attackable.Combatant)
	return !creature.CanDealDamage(cb)
}

// stopEffectsBySkillID removes every active effect in list owned by the
// skill id, used to drop a stale toggle or prior cast's effects before a
// fresh copy is applied so the same skill doesn't stack on itself.
func stopEffectsBySkillID(list *effect.List, id modelskill.ID) {
	removeMatching(list, 0, func(e *effect.Effect) bool {
		return e.Skill.ID == id
	})
}

// firstEffectByID returns the first active effect in list whose owning
// skill matches id, or nil if list is nil or has none.
func firstEffectByID(list *effect.List, id modelskill.ID) *effect.Effect {
	if list == nil {
		return nil
	}
	for _, e := range list.All() {
		if e.Skill.ID == id {
			return e
		}
	}
	return nil
}

// ActiveEffect reports whether target's live effect list currently holds an
// active instance of skill id — the caller-side lookup a toggle skill's
// on/off decision needs before driving cast.Controller.CastToggle.
func ActiveEffect(target Actor, id modelskill.ID) bool {
	t, ok := target.(effect.Actor)
	if !ok {
		return false
	}
	return firstEffectByID(t.EffectList(), id) != nil
}

// StopEffect removes target's active instance of skill id from its live
// effect list, if one exists, running that instance's exit hook. This is
// how deactivating an already-active toggle turns it off.
func StopEffect(target Actor, id modelskill.ID) {
	t, ok := target.(effect.Actor)
	if !ok {
		return
	}
	stopEffectsBySkillID(t.EffectList(), id)
}

// removeMatching removes every effect in list for which remove returns
// true, stopping once limit removals have happened when limit > 0
// (matching a "cancel at most N effects" cap; limit <= 0 means unlimited).
func removeMatching(list *effect.List, limit int, remove func(*effect.Effect) bool) {
	if list == nil {
		return
	}
	removed := 0
	for _, e := range list.All() {
		if !remove(e) {
			continue
		}
		list.Remove(e)
		removed++
		if limit > 0 && removed >= limit {
			return
		}
	}
}

// applySelfEffects refreshes and (re)applies def's self-targeted effects on
// caster: an existing self effect from the same skill is dropped first, so
// re-triggering the skill doesn't stack a duplicate of it.
func applySelfEffects(cast Cast, def modelskill.Definition) {
	if len(def.SelfEffects) == 0 {
		return
	}
	if target, ok := cast.Caster.(effect.Actor); ok {
		list := target.EffectList()
		if e := firstEffectByID(list, def.ID); e != nil && e.Template.Self {
			list.Remove(e)
		}
	}
	applyCasterSelfEffects(cast, def)
}
