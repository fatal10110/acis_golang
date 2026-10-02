package skill

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// fusionHandler grows an existing live Fusion effect of a FUSION cast's
// triggered skill in place, or applies that triggered skill fresh. The live
// entry is StartFusion, which a player's fusion cast path calls as its
// channel opens; no cast reaches Use through the hit-time dispatch (a
// player's FUSION cast never hits, and AIController skips FUSION for every
// other caster), so Use is kept only so the dispatch table covers FUSION.
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
	for _, obj := range cast.Targets {
		if target, ok := obj.(effect.Actor); ok {
			h.start(cast.Caster, target, cast.Skill)
		}
	}
}

// start grows target's live effect of castSkill's triggered skill, or
// applies that triggered skill fresh.
func (h fusionHandler) start(caster Creature, target effect.Actor, castSkill modelskill.Definition) {
	list := target.EffectList()
	if list == nil {
		return
	}
	triggeredID := modelskill.ID(castSkill.TriggeredID)
	if e := firstEffectByID(list, triggeredID); e != nil {
		maxLevel := h.defs.MaxLevel(triggeredID)
		e.IncreaseEffect(list, maxLevel, func(level int) {
			h.applyTriggered(caster, target, triggeredID, level)
		})
		return
	}
	h.applyTriggered(caster, target, triggeredID, castSkill.TriggeredLevel)
}

// StartFusion lands castSkill's force on effected as its FUSION channel
// opens, the way Use does, and nothing more: the channel's start is no
// skill use, so it flags no one, rolls no chance skill and notifies no
// attacked target.
func StartFusion(defs Definitions, caster Creature, effected attackable.Combatant, castSkill modelskill.Definition) {
	target, ok := effected.(effect.Actor)
	if !ok || defs == nil {
		return
	}
	fusionHandler{defs: defs}.start(caster, target, castSkill)
}

func (h fusionHandler) applyTriggered(caster Creature, effected Actor, triggeredID modelskill.ID, level int) {
	def, ok := h.defs.Definition(modelskill.Ref{ID: triggeredID, Level: level})
	if !ok {
		return
	}
	applyEffects(caster, effected, def, def.Effects)
}

// DecreaseFusion removes one level from the target's triggered fusion effect.
// It is called when the owning FUSION cast channel ends or aborts.
func DecreaseFusion(defs Definitions, caster Creature, effected attackable.Combatant, castSkill modelskill.Definition) {
	target, ok := effected.(effect.Actor)
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
		h.applyTriggered(caster, effected, triggeredID, level)
	})
}
