package cast

import (
	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
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
	// Sink, when set, delivers each skill-handler message as the handler
	// produces it, in place of EffectResult.Messages, so the message keeps
	// its place among the frames the hit's own state changes send.
	Sink handlerskill.MessageSink
	// Chance fires the chance procs a landing skill sets off on its caster
	// and targets; nil fires none.
	Chance *ChanceProcs
}

// EffectResult reports whether effect dispatch reached a skill handler and
// which player-visible outcomes that handler produced.
type EffectResult struct {
	Handled           bool
	Messages          []any
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
	handlerskill.Creature
	// TestCursesOnSkillSee applies the raid skill-see curse and reports
	// whether it blocked the skill.
	TestCursesOnSkillSee(def modelskill.Definition, targets []skilltarget.Actor) bool
	// NotePvPSkillTargets records a resolved cast's targets for PvP flagging.
	NotePvPSkillTargets(targets []attackable.Combatant, offensive bool, skillType string)
}

// skillAttackedTarget is an NPC that reacts to being hit by an offensive
// debuff or a skill with aggro points.
type skillAttackedTarget interface {
	SkillAttacked(caster attackable.Combatant, def modelskill.Definition)
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
	return nonEmpty(handler.Targets(caster, selected, &def))
}

func nonEmpty(affected []skilltarget.Actor) ([]skilltarget.Actor, bool) {
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

// ResolveAffected resolves def's affected set at an AI cast's launch, for a
// caller that must broadcast it (MagicSkillLaunched) and then reuse that
// exact list — not a fresh resolution — when Hit dispatches effects: the
// launch assigns the target list once and the hit timer's skill call
// reuses that same list rather than re-deriving it. The target conditions are not
// judged again here: a playable caster's were checked when the cast
// committed, with its CTRL state, and an NPC caster's are never checked.
// ok is false if resolution failed for any reason (unresolvable
// caster/target, no registered handler, or an empty affected set);
// affected is nil in that case.
func ResolveAffected(handlers EffectHandlers, caster skilltarget.Actor, resolved Target, def modelskill.Definition) (affected []skilltarget.Actor, ok bool) {
	affected, ok = resolveLaunchTargets(handlers, caster, resolved, def)
	if !ok {
		return nil, false
	}
	return nonEmpty(affected)
}

// resolveLaunchTargets is ResolveAffected keeping an empty affected set:
// ok is true whenever def's target handler ran, whatever it found.
func resolveLaunchTargets(handlers EffectHandlers, caster skilltarget.Actor, resolved Target, def modelskill.Definition) (affected []skilltarget.Actor, ok bool) {
	if caster == nil || handlers.Targets == nil {
		return nil, false
	}
	selected, ok := resolved.(skilltarget.Actor)
	if !ok {
		return nil, false
	}
	handler, ok := handlers.Targets.Handler(def.Target)
	if !ok {
		return nil, false
	}
	return handler.Targets(caster, selected, &def), true
}

// ApplyResolvedEffectsResult dispatches def's effects to affected — an
// already-resolved target set, typically the one ResolveAffected returned
// at launch and frozen for reuse at Hit — instead of re-resolving from a
// single selection. See ResolveAffected's doc for why a caller needs this
// split.
func ApplyResolvedEffectsResult(handlers EffectHandlers, caster skilltarget.Actor, affected []skilltarget.Actor, def modelskill.Definition) EffectResult {
	return ApplyResolvedItemEffectsResult(handlers, caster, affected, def, nil)
}

// ApplyResolvedItemEffectsResult is ApplyResolvedEffectsResult for a cast
// that carries the item it was cast with (e.g. a seed's SOW cast), threading
// item through to the skill handler as handlerskill.Cast.Item.
//
// When the launch resolved no target, no effect is dispatched, but a
// player's skill, or its summon's, is still seen by the NPCs around its
// caster (castWithoutTargets).
func ApplyResolvedItemEffectsResult(handlers EffectHandlers, caster skilltarget.Actor, affected []skilltarget.Actor, def modelskill.Definition, item any) EffectResult {
	if handlers.Skills == nil {
		return EffectResult{}
	}
	if len(affected) == 0 {
		castWithoutTargets(caster, def)
		return EffectResult{}
	}
	return dispatchEffects(handlers, caster, affected, def, item)
}

// castWithoutTargets is the skill call of a cast whose launch resolved no
// target: once the raid curses, which have no target to test, let it
// through, a player's skill, or its summon's, reaches every NPC around the
// caster that watches spells, with no targets.
func castWithoutTargets(caster skilltarget.Actor, def modelskill.Definition) {
	castCaster, ok := caster.(SkillCaster)
	if !ok {
		return
	}
	player, isPet, acting := actingPlayer(caster)
	if !acting {
		return
	}
	if def.Activation != modelskill.ActivationToggle && castCaster.TestCursesOnSkillSee(def, nil) {
		return
	}
	seeSpell(castCaster, player, def, nil, isPet)
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
	handlers.Chance.skillHit(castCaster, affected, def)
	if def.Offensive {
		switch def.SkillType {
		case "AGGREDUCE", "AGGREMOVE", "AGGREDUCE_CHAR":
		default:
			for _, target := range affected {
				if recorder, ok := target.(interface{ RecordAttacker(attackable.Combatant) }); ok {
					recorder.RecordAttacker(castCaster)
				}
				if attacked, ok := target.(interface{ NotifyAttacked(attackable.Combatant) }); ok {
					attacked.NotifyAttacked(castCaster)
				}
			}
		}
	}

	if def.Overhit && caster.Kind().Playable() {
		for _, t := range affected {
			if creature, ok := t.(handlerskill.Creature); ok {
				creature.EnableOverhit()
			}
		}
	}

	result, ok := handlers.Skills.UseResult(handlerskill.Cast{
		Caster:  castCaster,
		Skill:   def,
		Targets: castTargets,
		Item:    item,
		Sink:    handlers.Sink,
	})
	// An offensive debuff, or an offensive skill with aggro points, calls
	// each NPC it hit once its effects applied, dead or not.
	if def.Offensive && (def.Debuff || def.AggroPoints > 0) {
		for _, target := range affected {
			if attacked, ok := target.(skillAttackedTarget); ok {
				attacked.SkillAttacked(castCaster, def)
			}
		}
	}
	// PvP flagging follows the skill's effects, so a skill that makes its
	// caster a PKer flags it again after the kill ended its flag.
	castCaster.NotePvPSkillTargets(notifyTargets, def.Offensive, def.SkillType)
	// A player's skill, or its summon's, then reaches the NPCs whose
	// scripts watch spells: the targets it landed on, then the NPCs around
	// the caster.
	if player, isPet, acting := actingPlayer(caster); acting {
		spelled(player, def, affected)
		seeSpell(castCaster, player, def, affected, isPet)
	}
	if !ok {
		return EffectResult{}
	}
	return effectResultOf(result)
}

// CallSkill lands def's effects from caster on target alone, through def's
// skill handler, as a script makes a creature use a skill: no target
// resolution, cost, chance procs, aggression or PvP flagging. Handlers'
// Sink takes the handler's messages.
func CallSkill(handlers EffectHandlers, caster handlerskill.Creature, target handlerskill.Actor, def modelskill.Definition) EffectResult {
	if handlers.Skills == nil {
		return EffectResult{}
	}
	result, ok := handlers.Skills.UseResult(handlerskill.Cast{Caster: caster, Skill: def, Targets: []handlerskill.Actor{target}, Sink: handlers.Sink})
	if !ok {
		return EffectResult{}
	}
	return effectResultOf(result)
}

// effectResultOf is the caster-visible part of a handler's result.
func effectResultOf(result handlerskill.Result) EffectResult {
	return EffectResult{
		Handled:           true,
		Messages:          result.Messages,
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
