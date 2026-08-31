package cast

import (
	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
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
	Lethals           []handlerskill.Lethal
	Dodges            []handlerskill.Dodge
	Resisted          []handlerskill.Resisted
	MagicResists      []handlerskill.MagicResist
	CubicAdded        bool
	CubicTargets      []handlerskill.Actor
	CubicAddedTargets []handlerskill.Actor
	CubicTouched      bool
	CubicID           cubic.ID
}

type pvpSkillNotifier interface {
	NotePvPSkillTargets([]creature.DeathActor, bool, string)
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
func ApplyEffects(handlers EffectHandlers, caster skilltarget.Creature, resolved Target, def modelskill.Definition) bool {
	return ApplyEffectsResult(handlers, caster, resolved, def).Handled
}

// ApplyEffectsResult resolves def's affected targets and returns any
// caster-visible result the selected skill handler produced.
func ApplyEffectsResult(handlers EffectHandlers, caster skilltarget.Creature, resolved Target, def modelskill.Definition) EffectResult {
	return applyEffectsResult(handlers, caster, resolved, def, nil)
}

// ApplyItemEffectsResult is ApplyEffectsResult for a skill cast that carries
// the item it was cast with (e.g. a pet-collar's SUMMON_CREATURE cast),
// threading item through to the skill handler as handlerskill.Cast.Item —
// ApplyEffectsResult itself never sets Item, matching every other skill
// type, which has no use for it.
func ApplyItemEffectsResult(handlers EffectHandlers, caster skilltarget.Creature, resolved Target, def modelskill.Definition, item any) EffectResult {
	return applyEffectsResult(handlers, caster, resolved, def, item)
}

func resolveAffected(handlers EffectHandlers, caster skilltarget.Creature, resolved Target, def modelskill.Definition) ([]skilltarget.Creature, bool) {
	if caster == nil || handlers.Targets == nil {
		return nil, false
	}
	selected, _ := resolved.(skilltarget.Creature)

	handler, ok := handlers.Targets.Handler(def.Target)
	if !ok || !handler.CanCast(caster, selected, &def, false) {
		return nil, false
	}

	affected := handler.Targets(caster, selected, &def)
	if len(affected) == 0 {
		return nil, false
	}
	return affected, true
}

func applyEffectsResult(handlers EffectHandlers, caster skilltarget.Creature, resolved Target, def modelskill.Definition, item any) EffectResult {
	if handlers.Skills == nil {
		return EffectResult{}
	}
	affected, ok := resolveAffected(handlers, caster, resolved, def)
	if !ok {
		return EffectResult{}
	}
	return dispatchEffects(handlers, caster, affected, def, item)
}

// ResolveAffected exposes the same target-resolution surface
// applyEffectsResult uses, for a caller that must broadcast the affected
// set (e.g. MagicSkillLaunched) at launch and then reuse that exact,
// already-resolved list — not a fresh resolution — when Hit dispatches
// effects, matching CreatureCast.java: onMagicLaunch assigns
// `_targets = _skill.getTargetList(...)` once (:232) and the hit timer's
// `callSkill(_skill, _targets, _item)` (:291, NpcCast.java:52) reuses that
// same field rather than re-deriving it. ok is false if resolution failed
// for any reason (unresolvable caster/target, no registered handler,
// CanCast rejection, or an empty affected set); affected is nil in that
// case.
func ResolveAffected(handlers EffectHandlers, caster skilltarget.Creature, resolved Target, def modelskill.Definition) (affected []skilltarget.Creature, ok bool) {
	return resolveAffected(handlers, caster, resolved, def)
}

// ApplyResolvedEffectsResult dispatches def's effects to affected — an
// already-resolved target set, typically the one ResolveAffected returned
// at launch and frozen for reuse at Hit — instead of re-resolving from a
// single selection. See ResolveAffected's doc for why a caller needs this
// split.
func ApplyResolvedEffectsResult(handlers EffectHandlers, caster skilltarget.Creature, affected []skilltarget.Creature, def modelskill.Definition) EffectResult {
	if handlers.Skills == nil || len(affected) == 0 {
		return EffectResult{}
	}
	return dispatchEffects(handlers, caster, affected, def, nil)
}

func dispatchEffects(handlers EffectHandlers, caster skilltarget.Creature, affected []skilltarget.Creature, def modelskill.Definition, item any) EffectResult {
	// caster already satisfies skilltarget.Creature, a strict superset of
	// handlerskill.Actor, so no runtime guard is needed here.
	castCaster := handlerskill.Actor(caster)
	castTargets := make([]handlerskill.Actor, len(affected))
	for i, t := range affected {
		castTargets[i] = t
	}
	if notifier, ok := caster.(pvpSkillNotifier); ok {
		notifyTargets := make([]creature.DeathActor, len(castTargets))
		for i, t := range castTargets {
			notifyTargets[i] = t
		}
		notifier.NotePvPSkillTargets(notifyTargets, def.Offensive, def.SkillType)
	}

	result, ok := handlers.Skills.UseResult(handlerskill.Cast{
		Caster:  castCaster,
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
		Lethals:           result.Lethals,
		Dodges:            result.Dodges,
		Resisted:          result.Resisted,
		MagicResists:      result.MagicResists,
		CubicAdded:        result.CubicAdded,
		CubicTargets:      result.CubicTargets,
		CubicAddedTargets: result.CubicAddedTargets,
		CubicTouched:      result.CubicTouched,
		CubicID:           result.CubicID,
	}
}
