package skill

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// effectListTarget is implemented by anything that owns a live effect list:
// a target an effect-applying or effect-cancelling skill can act on.
type effectListTarget interface {
	Actor
	EffectList() *effect.List
}

type effectSuccessSource interface {
	EffectSuccessInput(creature.DeathActor, modelskill.Definition, modelskill.EffectTemplate, bool, formulas.ShieldDefense) (formulas.SkillSuccessInput, bool)
}

type positionedActor interface {
	Position() (int, int, int)
}

type invulnerableEffected interface {
	Invul() bool
}

// applyEffects instantiates each of templates and adds it to effected's
// effect list, attributed to effector. def carries the owning skill's
// identity and stacking classification. A template naming an effect core
// this port hasn't wired yet (see effect.New) is skipped rather than
// failing the whole batch, matching how partially-modeled skill data
// degrades elsewhere in this package.
func applyEffects(effector, effected Actor, def modelskill.Definition, templates []modelskill.EffectTemplate) {
	applyEffectsWithLanding(effector, effected, def, templates, formulas.ShieldFailed, false)
}

func applyCastEffects(cast Cast, effected Actor, def modelskill.Definition, templates []modelskill.EffectTemplate) {
	cast.reportResisted(effected, def, applyEffectsWithLanding(cast.Caster, effected, def, templates, formulas.ShieldFailed, false))
}

func applyEffectsWithLanding(effector, effected Actor, def modelskill.Definition, templates []modelskill.EffectTemplate, shield formulas.ShieldDefense, bss bool) (resisted int) {
	if len(templates) == 0 {
		return 0
	}
	if !withinEffectRange(effector, effected, def.EffectRange) {
		return 0
	}
	if offensiveEffectApplyBlocked(effector, effected, def) {
		return 0
	}
	target, ok := effected.(effectListTarget)
	if !ok {
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
	source, canRoll := effected.(effectSuccessSource)
	for _, tmpl := range templates {
		if tmpl.EffectPowerSet && tmpl.EffectPower >= 0 {
			if !canRoll {
				if tmpl.Icon {
					resisted++
				}
				continue
			}
			in, ok := source.EffectSuccessInput(effector, def, tmpl, bss, shield)
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
			self, ok := effector.(effectListTarget)
			if !ok || self.EffectList() == nil {
				continue
			}
			owner = self.EffectList()
		}
		e.Effector = effector
		e.Effected = effected
		owner.Add(e)
	}
	return resisted
}

// withinEffectRange mirrors L2Skill.getEffects' landing-time 3D radius
// check. Actors without a modeled position stay permissive like other
// optional handler capabilities.
func withinEffectRange(effector, effected Actor, effectRange int) bool {
	if effectRange <= 0 || effector == nil || effector.ObjectID() == effected.ObjectID() {
		return true
	}
	effectorPos, ok := effector.(positionedActor)
	if !ok {
		return true
	}
	effectedPos, ok := effected.(positionedActor)
	if !ok {
		return true
	}
	ax, ay, az := effectorPos.Position()
	bx, by, bz := effectedPos.Position()
	return location.Location{X: ax, Y: ay, Z: az}.Distance3D(location.Location{X: bx, Y: by, Z: bz}) < float64(effectRange)
}

// offensiveEffectApplyBlocked is the landing-time refuse for offensive and
// debuff skills aimed at someone else: target invulnerability, or a caster
// not permitted to deal damage. Self-applies skip both.
func offensiveEffectApplyBlocked(effector, effected Actor, def modelskill.Definition) bool {
	if !def.Offensive && !def.Debuff {
		return false
	}
	if effector != nil && effected != nil && effector.ObjectID() == effected.ObjectID() {
		return false
	}
	if inv, ok := effected.(invulnerableEffected); ok && inv.Invul() {
		return true
	}
	return !creature.CanDealDamage(effector)
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
	t, ok := target.(effectListTarget)
	if !ok {
		return false
	}
	return firstEffectByID(t.EffectList(), id) != nil
}

// StopEffect removes target's active instance of skill id from its live
// effect list, if one exists, running that instance's exit hook. This is
// how deactivating an already-active toggle turns it off.
func StopEffect(target Actor, id modelskill.ID) {
	t, ok := target.(effectListTarget)
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
	if target, ok := cast.Caster.(effectListTarget); ok {
		list := target.EffectList()
		if e := firstEffectByID(list, def.ID); e != nil && e.Template.Self {
			list.Remove(e)
		}
	}
	applyCastEffects(cast, cast.Caster, def, def.SelfEffects)
}
