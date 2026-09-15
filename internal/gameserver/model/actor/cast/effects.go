package cast

import (
	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// EffectHandlers groups the target-resolution and skill-effect handler
// registries a resolved cast needs to affect its targets.
type EffectHandlers struct {
	Targets *skilltarget.Registry
	Skills  *handlerskill.Registry
	// OnHitResult, when set, is wired onto AIController.OnHitResult for a
	// caster with no live connection of its own (a hostile NPC) so its
	// target-addressed messages (MagicResist, ManaDrain, ...) still reach a
	// real online target. It is threaded here rather than as its own
	// constructor parameter because EffectHandlers is already the bundle
	// carried from boot wiring down to the AIController that needs it.
	OnHitResult func(EffectResult)
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
	ManaDamageMissed  int
	ManaDrains        []handlerskill.ManaDrain
	OpponentMPReduced []int32
	CubicAdded        bool
	CubicTargets      []handlerskill.Actor
	CubicAddedTargets []handlerskill.Actor
	CubicTouched      bool
	CubicID           cubic.ID
}

// SkillCaster is a creature whose skill effects the cast pipeline dispatches.
type SkillCaster interface {
	handlerskill.Caster
	// TestCursesOnSkillSee applies the raid skill-see curse and reports
	// whether it blocked the skill.
	TestCursesOnSkillSee(def modelskill.Definition, targets []skilltarget.Actor) bool
	// NotePvPSkillTargets records a resolved cast's targets for PvP flagging.
	NotePvPSkillTargets(targets []attackable.Combatant, offensive bool, skillType string)
}

// ApplyEffects resolves def's affected target set from caster and the
// already cast-validated single selection, then routes the skill's effects
// to the resolved set. It reports whether a skill handler actually ran.
//
// caster only needs to satisfy the target-resolution surface
// (skilltarget.Actor), not any player-specific type, so this is the same
// resolution and dispatch path any caster drives — a live player today, and
// eventually an NPC- or summon-initiated cast once that scheduling exists —
// rather than a player-only shortcut. A caster or resolved selection that
// doesn't satisfy the target-resolution surfaces, a target type with no
// registered handler, a target type that rejects the cast, or a skill type
// with no registered effect handler all result in no effect applied; that
// mirrors the graceful degradation the effect handlers already use for
// actor state this port hasn't modeled yet, rather than failing the caller.
func ApplyEffects(handlers EffectHandlers, caster skilltarget.Actor, resolved Target, def modelskill.Definition) bool {
	return ApplyEffectsResult(handlers, caster, resolved, def).Handled
}

// ApplyEffectsResult resolves def's affected targets and returns any
// caster-visible result the selected skill handler produced.
func ApplyEffectsResult(handlers EffectHandlers, caster skilltarget.Actor, resolved Target, def modelskill.Definition) EffectResult {
	return applyEffectsResult(handlers, caster, resolved, def, nil)
}

// ApplyItemEffectsResult is ApplyEffectsResult for a skill cast that carries
// the item it was cast with (e.g. a pet-collar's SUMMON_CREATURE cast),
// threading item through to the skill handler as handlerskill.Cast.Item —
// ApplyEffectsResult itself never sets Item, matching every other skill
// type, which has no use for it.
func ApplyItemEffectsResult(handlers EffectHandlers, caster skilltarget.Actor, resolved Target, def modelskill.Definition, item any) EffectResult {
	return applyEffectsResult(handlers, caster, resolved, def, item)
}

func resolveAffected(handlers EffectHandlers, caster skilltarget.Actor, resolved Target, def modelskill.Definition) ([]skilltarget.Actor, bool) {
	if caster == nil || handlers.Targets == nil {
		return nil, false
	}
	selected, _ := resolved.(skilltarget.Actor)

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

func applyEffectsResult(handlers EffectHandlers, caster skilltarget.Actor, resolved Target, def modelskill.Definition, item any) EffectResult {
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
func ResolveAffected(handlers EffectHandlers, caster skilltarget.Actor, resolved Target, def modelskill.Definition) (affected []skilltarget.Actor, ok bool) {
	return resolveAffected(handlers, caster, resolved, def)
}

// ApplyResolvedEffectsResult dispatches def's effects to affected — an
// already-resolved target set, typically the one ResolveAffected returned
// at launch and frozen for reuse at Hit — instead of re-resolving from a
// single selection. See ResolveAffected's doc for why a caller needs this
// split.
func ApplyResolvedEffectsResult(handlers EffectHandlers, caster skilltarget.Actor, affected []skilltarget.Actor, def modelskill.Definition) EffectResult {
	if handlers.Skills == nil || len(affected) == 0 {
		return EffectResult{}
	}
	return dispatchEffects(handlers, caster, affected, def, nil)
}

func dispatchEffects(handlers EffectHandlers, caster skilltarget.Actor, affected []skilltarget.Actor, def modelskill.Definition, item any) EffectResult {
	// Only creatures cast skills; a door never reaches the handlers as a caster.
	castCaster, ok := caster.(SkillCaster)
	if !ok {
		return EffectResult{}
	}
	if def.Activation != modelskill.ActivationToggle && castCaster.TestCursesOnSkillSee(def, affected) {
		return EffectResult{}
	}
	castTargets := make([]handlerskill.Actor, len(affected))
	for i, t := range affected {
		castTargets[i] = t
	}
	// Doors never take part in PvP flagging, so only creature targets are
	// reported.
	notifyTargets := make([]attackable.Combatant, 0, len(affected))
	for _, t := range affected {
		if c, ok := t.(attackable.Combatant); ok {
			notifyTargets = append(notifyTargets, c)
		}
	}
	castCaster.NotePvPSkillTargets(notifyTargets, def.Offensive, def.SkillType)

	if def.Overhit && caster.Kind().Playable() {
		for _, t := range affected {
			// Only hostile NPCs track overhit damage.
			if hostile, ok := t.(*npc.Hostile); ok {
				hostile.EnableOverhit()
			}
		}
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
		ManaDamageMissed:  result.ManaDamageMissed,
		ManaDrains:        result.ManaDrains,
		OpponentMPReduced: result.OpponentMPReduced,
		CubicAdded:        result.CubicAdded,
		CubicTargets:      result.CubicTargets,
		CubicAddedTargets: result.CubicAddedTargets,
		CubicTouched:      result.CubicTouched,
		CubicID:           result.CubicID,
	}
}
