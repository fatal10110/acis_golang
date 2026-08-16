package skill

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// fusionHandler ports FusionSkill's constructor (java FusionSkill.java:27-43):
// a FUSION-skillType cast grows an existing live Fusion effect from its
// triggered skill in place, or applies that triggered skill fresh.
type fusionHandler struct {
	defs Definitions
}

func (fusionHandler) Types() []string { return []string{"FUSION"} }

// Use ports FusionSkill(caster, target, skill): each target already carrying
// a live effect owned by the cast skill's TriggeredID grows it via
// IncreaseEffect, capped at that triggered skill's max level. A target with
// no such effect gets the triggered skill's effects applied fresh. The
// channel-abort DecreaseForce path (FusionSkill.onCastAbort, gated on a
// geo-range check) is wired via cast/schedule.go's ScheduleFusion, not this
// Use handler — a Use handler does not run abort cleanup.
func (h fusionHandler) Use(cast Cast) {
	if h.defs == nil {
		return
	}
	triggeredID := modelskill.ID(cast.Skill.TriggeredID)

	for _, obj := range cast.Targets {
		target, ok := obj.(effectListTarget)
		if !ok {
			continue
		}
		list := target.EffectList()
		if list == nil {
			continue
		}

		if e := firstEffectByID(list, triggeredID); e != nil {
			maxLevel := h.defs.MaxLevel(triggeredID)
			e.IncreaseEffect(list, maxLevel, func(level int) {
				h.applyTriggered(cast.Caster, target, triggeredID, level)
			})
			continue
		}

		h.applyTriggered(cast.Caster, target, triggeredID, cast.Skill.TriggeredLevel)
	}
}

func (h fusionHandler) applyTriggered(caster, effected Actor, triggeredID modelskill.ID, level int) {
	def, ok := h.defs.Definition(modelskill.Ref{ID: triggeredID, Level: level})
	if !ok {
		return
	}
	applyEffects(caster, effected, def, def.Effects)
}

// DecreaseFusion removes one level from the target's triggered fusion effect.
// It is called when the owning FUSION cast channel ends or aborts. effected
// takes creature.DeathActor because the channel's caller holds its target as
// a world-object selection rather than a resolved cast participant; a
// selection that owns no effect list is dropped below either way.
func DecreaseFusion(defs Definitions, caster Actor, effected creature.DeathActor, castSkill modelskill.Definition) {
	target, ok := effected.(effectListTarget)
	if !ok || defs == nil {
		return
	}
	list := target.EffectList()
	triggeredID := modelskill.ID(castSkill.TriggeredID)
	e := firstEffectByID(list, triggeredID)
	if e == nil {
		return
	}
	h := fusionHandler{defs: defs}
	e.DecreaseForce(list, func(level int) {
		h.applyTriggered(caster, target, triggeredID, level)
	})
}
