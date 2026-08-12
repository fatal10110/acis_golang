package cast

import (
	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// EffectHandlers groups the target-resolution and skill-effect handler
// registries a resolved cast needs to affect its targets.
type EffectHandlers struct {
	Targets *skilltarget.Registry
	Skills  *handlerskill.Registry
}

// EffectResult reports whether effect dispatch reached a skill handler and
// which player-visible outcomes that handler produced.
type EffectResult struct {
	Handled           bool
	AttackFailed      int
	Counterattacks    []handlerskill.Counterattack
	Dodges            []handlerskill.Dodge
	CubicAdded        bool
	CubicTargets      []any
	CubicAddedTargets []any
	CubicTouched      bool
	CubicID           cubic.ID
}

type pvpSkillNotifier interface {
	NotePvPSkillTargets([]any, bool, string)
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
	return ApplyEffectsResult(handlers, caster, resolved, def).Handled
}

// ApplyEffectsResult resolves def's affected targets and returns any
// caster-visible result the selected skill handler produced.
func ApplyEffectsResult(handlers EffectHandlers, caster any, resolved Target, def modelskill.Definition) EffectResult {
	return applyEffectsResult(handlers, caster, resolved, def, nil)
}

// ApplyItemEffectsResult is ApplyEffectsResult for a skill cast that carries
// the item it was cast with (e.g. a pet-collar's SUMMON_CREATURE cast),
// threading item through to the skill handler as handlerskill.Cast.Item —
// ApplyEffectsResult itself never sets Item, matching every other skill
// type, which has no use for it.
func ApplyItemEffectsResult(handlers EffectHandlers, caster any, resolved Target, def modelskill.Definition, item any) EffectResult {
	return applyEffectsResult(handlers, caster, resolved, def, item)
}

// ResolveTargetIDs returns the object IDs def's affected target set would
// resolve to from caster and resolved — the same target-resolution surface
// applyEffectsResult uses, exposed for callers (the launch-phase
// MagicSkillLaunched broadcast) that need the affected list before Hit
// applies effects. Returns nil if resolution fails for any reason
// (unresolvable caster/target, no registered handler, or CanCast rejects
// the cast).
func ResolveTargetIDs(handlers EffectHandlers, caster any, resolved Target, def modelskill.Definition) []int32 {
	affected, ok := resolveAffected(handlers, caster, resolved, def)
	if !ok {
		return nil
	}
	ids := make([]int32, len(affected))
	for i, t := range affected {
		ids[i] = t.ObjectID()
	}
	return ids
}

func resolveAffected(handlers EffectHandlers, caster any, resolved Target, def modelskill.Definition) ([]skilltarget.Creature, bool) {
	casterCreature, ok := caster.(skilltarget.Creature)
	if !ok || handlers.Targets == nil {
		return nil, false
	}
	selected, _ := resolved.(skilltarget.Creature)

	handler, ok := handlers.Targets.Handler(def.Target)
	if !ok || !handler.CanCast(casterCreature, selected, &def, false) {
		return nil, false
	}

	affected := handler.Targets(casterCreature, selected, &def)
	if len(affected) == 0 {
		return nil, false
	}
	return affected, true
}

func applyEffectsResult(handlers EffectHandlers, caster any, resolved Target, def modelskill.Definition, item any) EffectResult {
	if handlers.Skills == nil {
		return EffectResult{}
	}
	affected, ok := resolveAffected(handlers, caster, resolved, def)
	if !ok {
		return EffectResult{}
	}

	castTargets := make([]any, len(affected))
	for i, t := range affected {
		castTargets[i] = t
	}
	if notifier, ok := caster.(pvpSkillNotifier); ok {
		notifier.NotePvPSkillTargets(castTargets, def.Offensive, def.SkillType)
	}

	result, ok := handlers.Skills.UseResult(handlerskill.Cast{
		Caster:  caster,
		Skill:   def,
		Targets: castTargets,
		Item:    item,
	})
	if !ok {
		return EffectResult{}
	}
	return EffectResult{
		Handled:           true,
		AttackFailed:      result.AttackFailed,
		Counterattacks:    result.Counterattacks,
		Dodges:            result.Dodges,
		CubicAdded:        result.CubicAdded,
		CubicTargets:      result.CubicTargets,
		CubicAddedTargets: result.CubicAddedTargets,
		CubicTouched:      result.CubicTouched,
		CubicID:           result.CubicID,
	}
}
