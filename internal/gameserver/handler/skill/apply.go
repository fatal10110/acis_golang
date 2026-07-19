package skill

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// effectListTarget is implemented by anything that owns a live effect list:
// a target an effect-applying or effect-cancelling skill can act on.
type effectListTarget interface {
	EffectList() *effect.List
}

// applyEffects instantiates each of templates and adds it to effected's
// effect list, attributed to effector. def carries the owning skill's
// identity and stacking classification. A template naming an effect core
// this port hasn't wired yet (see effect.New) is skipped rather than
// failing the whole batch, matching how partially-modeled skill data
// degrades elsewhere in this package.
func applyEffects(effector, effected any, def modelskill.Definition, templates []modelskill.EffectTemplate) {
	if len(templates) == 0 {
		return
	}
	target, ok := effected.(effectListTarget)
	if !ok {
		return
	}
	list := target.EffectList()
	if list == nil {
		return
	}

	meta := effect.Skill{
		ID:                  def.ID,
		Level:               def.Level,
		SkillType:           def.SkillType,
		Debuff:              def.Debuff,
		Toggle:              def.Activation == modelskill.ActivationToggle,
		KillByDOT:           def.KillByDOT,
		CanBeDispelled:      def.CanBeDispelled,
		MagicLevel:          def.MagicLevel,
		AbnormalLevel:       def.AbnormalLevel,
		EffectAbnormalLevel: def.EffectAbnormalLevel,
		EffectType:          def.EffectType,
		MaxNegatedEffects:   def.MaxNegatedEffects,
		NegateLevel:         def.NegateLevel,
		NegateIDs:           def.NegateIDs,
		NegateTypes:         def.NegateTypes,
	}
	for _, tmpl := range templates {
		e, err := effect.New(meta, tmpl)
		if err != nil {
			continue
		}
		e.Effector = effector
		e.Effected = effected
		list.Add(e)
	}
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
func ActiveEffect(target any, id modelskill.ID) bool {
	t, ok := target.(effectListTarget)
	if !ok {
		return false
	}
	return firstEffectByID(t.EffectList(), id) != nil
}

// StopEffect removes target's active instance of skill id from its live
// effect list, if one exists, running that instance's exit hook. This is
// how deactivating an already-active toggle turns it off.
func StopEffect(target any, id modelskill.ID) {
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
func applySelfEffects(caster any, def modelskill.Definition) {
	if len(def.SelfEffects) == 0 {
		return
	}
	if target, ok := caster.(effectListTarget); ok {
		list := target.EffectList()
		if e := firstEffectByID(list, def.ID); e != nil && e.Template.Self {
			list.Remove(e)
		}
	}
	applyEffects(caster, caster, def, def.SelfEffects)
}
