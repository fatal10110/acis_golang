package cast

import (
	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// EffectHandlers groups the target-resolution and skill-effect handler
// registries a resolved cast needs to affect its targets.
type EffectHandlers struct {
	Targets *skilltarget.Registry
	Skills  *handlerskill.Registry
}

// ApplyEffects resolves def's affected target set from caster and the
// already cast-validated single selection, then routes the skill's effects
// to the resolved set. It reports whether a skill handler actually ran.
//
// caster only needs to satisfy the target-resolution surface
// (skilltarget.Creature), not any player-specific type, so this is the same
// resolution and dispatch path any caster drives — a live player today, and
// eventually an NPC- or summon-initiated cast once that scheduling exists —
// rather than a player-only shortcut. A caster or resolved selection that
// doesn't satisfy the target-resolution surfaces, a target type with no
// registered handler, a target type that rejects the cast, or a skill type
// with no registered effect handler all result in no effect applied; that
// mirrors the graceful degradation the effect handlers already use for
// actor state this port hasn't modeled yet, rather than failing the caller.
func ApplyEffects(handlers EffectHandlers, caster any, resolved Target, def modelskill.Definition) bool {
	casterCreature, ok := caster.(skilltarget.Creature)
	if !ok || handlers.Targets == nil || handlers.Skills == nil {
		return false
	}
	selected, _ := resolved.(skilltarget.Creature)

	handler, ok := handlers.Targets.Handler(def.Target)
	if !ok || !handler.CanCast(casterCreature, selected, &def, false) {
		return false
	}

	affected := handler.Targets(casterCreature, selected, &def)
	if len(affected) == 0 {
		return false
	}

	castTargets := make([]any, len(affected))
	for i, t := range affected {
		castTargets[i] = t
	}

	return handlers.Skills.Use(handlerskill.Cast{
		Caster:  caster,
		Skill:   def,
		Targets: castTargets,
	})
}
