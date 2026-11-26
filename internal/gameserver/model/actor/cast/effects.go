package cast

import (
	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
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
	// Corpses, when set, removes at once each corpse a player's or summon's
	// corpse skill lands on; nil leaves every corpse to its deadline.
	Corpses CorpseDecayer
}

// CorpseDecayer removes a corpse ahead of its decay deadline.
type CorpseDecayer interface {
	// ForceDecay removes actor's corpse when it has a pending decay and
	// reports whether it did.
	ForceDecay(actor task.DecayActor) bool
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
	NotePvPSkillTargets(targets []attackable.Combatant, offensive bool, skillType modelskill.SkillType)
}

// ApplyEffectsResult resolves def's affected target set from caster and the
// already cast-validated single selection, routes the skill's effects to it,
// and returns any caster-visible result the selected skill handler produced.
//
// caster only needs the target-resolution surface (skilltarget.Actor), so
// any caster drives this same path. A caster or selection that doesn't
// satisfy the target-resolution surfaces, a target type with no registered
// handler, a target type that rejects the cast, or a skill type with no
// registered effect handler all apply nothing (Handled stays false) rather
// than failing the caller.
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

// resolveLaunchTargets resolves def's affected set at an AI cast's launch,
// for a caller that broadcasts it (MagicSkillLaunched) and then reuses that
// exact list, not a fresh resolution, when Hit dispatches effects. The target
// conditions are not judged again here: a playable caster's were checked
// when the cast committed, with its CTRL state, and an NPC caster's are never
// checked. ok is true whenever def's target handler ran, even when it found
// no target; it is false for an unresolvable caster or selection or a target
// type with no registered handler.
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
// already-resolved target set, typically the one resolved at launch and
// frozen for reuse at Hit — instead of re-resolving from a single selection.
// See resolveLaunchTargets for why a caller needs this split.
func ApplyResolvedEffectsResult(handlers EffectHandlers, caster skilltarget.Actor, affected []skilltarget.Actor, def modelskill.Definition) EffectResult {
	return ApplyResolvedItemEffectsResult(handlers, caster, affected, def, nil)
}

// ApplyResolvedItemEffectsResult is ApplyResolvedEffectsResult for a cast
// that carries the item it was cast with (e.g. a seed's SOW cast), threading
// item through to the skill handler as handlerskill.Cast.Item.
//
// A launch that resolved no target still runs the skill's handler, with no
// targets: the handler's caster-side work (self effects, the shot it
// spends) happens whoever it hit, and a player's skill, or its summon's, is
// still seen by the NPCs around its caster.
func ApplyResolvedItemEffectsResult(handlers EffectHandlers, caster skilltarget.Actor, affected []skilltarget.Actor, def modelskill.Definition, item any) EffectResult {
	if handlers.Skills == nil {
		return EffectResult{}
	}
	return dispatchEffects(handlers, caster, affected, def, item)
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
	// Each target is armed for overhit before the procs the skill sets off
	// on it, so a proc that kills it already counts as an overhit. Only a
	// monster keeps the flag.
	procs := handlers.Chance.skillHitProcs(castCaster, def)
	overhit := def.Overhit && caster.Kind().Playable()
	for _, target := range affected {
		if creature, ok := target.(handlerskill.Creature); ok && overhit {
			creature.EnableOverhit()
		}
		if procs != nil {
			procs(target)
		}
	}
	if def.Offensive {
		switch def.SkillType {
		case modelskill.SkillTypeAggReduce, modelskill.SkillTypeAggRemove, modelskill.SkillTypeAggReduceChar:
		default:
			for _, target := range affected {
				target.RecordAttacker(castCaster)
				target.NotifyAttacked(castCaster)
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
			target.SkillAttacked(castCaster, def)
		}
	}
	// PvP flagging follows the skill's effects, so a skill that makes its
	// caster a PKer flags it again after the kill ended its flag.
	castCaster.NotePvPSkillTargets(notifyTargets, def.Offensive, def.SkillType)
	// A player's skill, or its summon's, then reaches the NPCs whose
	// scripts watch spells: the targets it landed on, which a corpse skill
	// then consumes, then the NPCs around the caster.
	if player, isPet, acting := actingPlayer(caster); acting {
		spelled(player, def, affected)
		gone := consumeCorpses(handlers.Corpses, def, affected)
		seeSpell(castCaster, player, def, affected, isPet, gone)
	}
	if !ok {
		return EffectResult{}
	}
	return effectResultOf(result)
}

// consumeCorpses removes at once the corpse of each of targets a
// corpse-mob skill landed on, a harvest excepted, and returns the ids of the
// corpses it removed. A corpse with no pending decay stays.
func consumeCorpses(corpses CorpseDecayer, def modelskill.Definition, targets []skilltarget.Actor) []int32 {
	if corpses == nil || def.SkillType == modelskill.SkillTypeHarvest {
		return nil
	}
	switch def.Target {
	case modelskill.TargetCorpseMob, modelskill.TargetAreaCorpseMob:
	default:
		return nil
	}
	var gone []int32
	for _, t := range targets {
		if corpse, ok := t.(task.DecayActor); ok && corpses.ForceDecay(corpse) {
			gone = append(gone, corpse.ObjectID())
		}
	}
	return gone
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
