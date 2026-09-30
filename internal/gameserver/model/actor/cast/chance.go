package cast

import (
	"cmp"
	"slices"
	"time"

	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// ChanceProcs fires chance-triggered skills: the skills a creature's
// chance-skill-trigger effects and, for a player, its passive chance skills
// cast on their own when the creature lands or takes a hit or a skill.
//
// A triggered skill has no cast time and no cast bar. The owner broadcasts
// its launch and its animation, and the skill handler runs at once, without
// the cast-hit steps a regular cast runs, so it never sets off further
// procs.
type ChanceProcs struct {
	Definitions Definitions
	Targets     *skilltarget.Registry
	Skills      *handlerskill.Registry
	// Deliver runs apply, which dispatches one triggered skill for caster,
	// and delivers its result the way a finished cast of that caster
	// delivers one. apply hands each skill-handler message to the sink it
	// is given the moment the handler produces it, so the message keeps its
	// place among the frames the skill's own state changes send at once; a
	// message the sink took is left out of the returned result. ownHit
	// reports that caster is the attacker or the skill caster whose hit set
	// the proc off, not the creature that was hit nor the effector of a
	// trigger effect: a player's hits and casts run on its own queue, so a
	// player proc with ownHit set runs there too. Nil runs apply without a
	// sink and drops the result.
	Deliver func(caster handlerskill.Creature, ownHit bool, apply func(sink handlerskill.MessageSink) EffectResult)
}

// ChanceConditionFailed is a triggered cast refused by one of its skill's
// <cond> clauses. Its message goes to the owner, as a failed cast's does.
type ChanceConditionFailed struct {
	Skill  modelskill.Definition
	Clause modelskill.ConditionClause
}

// ChanceWeaponNotAllowed is a triggered cast refused because its skill
// needs a weapon or shield type the owner does not hold. Its message goes
// to the owner, as a failed cast's does.
type ChanceWeaponNotAllowed struct {
	Skill modelskill.Definition
}

// WeaponSkillActivated is a weapon's on-magic skill about to run for a
// player caster. Its message goes to that player before the skill's own
// results.
type WeaponSkillActivated struct {
	Skill modelskill.Definition
}

// chanceEvents is the set of trigger events one proc point raises.
type chanceEvents uint8

func eventsOf(triggers ...modelskill.TriggerType) chanceEvents {
	var set chanceEvents
	for _, t := range triggers {
		set |= 1 << t
	}
	return set
}

func (s chanceEvents) has(t modelskill.TriggerType) bool { return s&(1<<t) != 0 }

// chanceOwner is a creature holding chance procs: the effects it registered
// through AddChanceTrigger. Its broadcasts show every proc it sets off.
type chanceOwner interface {
	chanceCaster
	skilltarget.Actor
	Roll(n int) int
	BroadcastSkillUse(targetID int32, targetX, targetY, targetZ int, skillID, level int32, hitTime, reuseDelay int)
	BroadcastSkillLaunched(skillID, level int32, targetIDs []int32)
}

// chanceCaster is the creature a triggered skill is cast by.
type chanceCaster interface {
	handlerskill.Creature
	SkillDisabled(key int32) bool
	DisableSkill(key int32, delay time.Duration)
}

// chanceSkillHolder is a player: its known passive skills that carry a
// chanceType are chance procs too.
type chanceSkillHolder interface {
	SkillLevels() player.SkillLevels
}

// AttackHit runs the procs of hit, a physical hit of attacker that dealt
// damage: the attacker's on-hit procs (and on-crit ones for a critical hit)
// against the target, its on-attacked procs against the target when the
// target reflected part of the damage, then the target's on-attacked procs
// against the attacker. A critical hit then casts the attacker's weapon
// on-critical skill. All of it runs on the attacker's queue, in the hit's
// own call, like the hit itself.
func (p *ChanceProcs) AttackHit(attacker any, hit event.HitLanded) {
	if p == nil {
		return
	}
	var target any = hit.Target
	landed := eventsOf(modelskill.TriggerOnHit)
	if hit.Crit {
		landed |= eventsOf(modelskill.TriggerOnCrit)
	}
	attacked := eventsOf(modelskill.TriggerOnAttacked, modelskill.TriggerOnAttackedHit)
	p.fire(attacker, true, target, landed)
	if hit.Reflected {
		p.fire(attacker, true, target, attacked)
	}
	p.fire(target, false, attacker, attacked)
	if hit.Crit {
		p.weaponCritSkill(attacker, target)
	}
}

// skillHit runs the procs of def landing on each of targets, before its
// handler runs: the caster's on-magic procs against the target, then the
// target's on-attacked procs against the caster when def deals damage.
func (p *ChanceProcs) skillHit(caster any, targets []skilltarget.Actor, def modelskill.Definition) {
	if p == nil {
		return
	}
	// A toggle and a fusion skill reach their handler directly, without the
	// cast-hit steps that raise these events; crafting raises none.
	if def.Activation == modelskill.ActivationToggle {
		return
	}
	switch def.SkillType {
	case "FUSION", "COMMON_CRAFT", "DWARVEN_CRAFT":
		return
	}
	var cast, taken chanceEvents
	switch {
	case def.IsDamage():
		cast = eventsOf(modelskill.TriggerOnMagicOffensive)
		taken = eventsOf(modelskill.TriggerOnAttacked)
	case !def.Offensive:
		cast = eventsOf(modelskill.TriggerOnMagicGood)
	}
	for _, target := range targets {
		if !target.Dead() {
			p.weaponMagicSkill(caster, target, def)
		}
		p.fire(caster, true, target, cast)
		p.fire(target, false, caster, taken)
	}
}

// weaponCaster is a creature whose active weapon may carry skills it casts
// on its own: on a critical hit, or on a skill it lands.
type weaponCaster interface {
	handlerskill.Creature
	Roll(n int) int
	// ActiveWeaponItem returns the weapon the creature attacks with, nil
	// when it has none.
	ActiveWeaponItem() *item.WeaponDetail
}

// weaponSkill returns the skill trigger of casterObj's weapon that pick
// selects, resolved, when the weapon carries one whose skill exists.
func (p *ChanceProcs) weaponSkill(casterObj any, pick func(*item.WeaponDetail) *item.SkillTrigger) (weaponCaster, *item.SkillTrigger, modelskill.Definition, bool) {
	caster, ok := casterObj.(weaponCaster)
	if !ok {
		return nil, nil, modelskill.Definition{}, false
	}
	weapon := caster.ActiveWeaponItem()
	if weapon == nil {
		return nil, nil, modelskill.Definition{}, false
	}
	trigger := pick(weapon)
	if trigger == nil {
		return nil, nil, modelskill.Definition{}, false
	}
	def, ok := p.definition(int(trigger.Skill.ID), int(trigger.Skill.Level))
	if !ok {
		return nil, nil, modelskill.Definition{}, false
	}
	return caster, trigger, def, true
}

// weaponChance reports whether caster wins trigger's activation chance; a
// trigger without one always activates.
func weaponChance(caster weaponCaster, trigger *item.SkillTrigger) bool {
	return trigger.Chance < 0 || caster.Roll(100) < int(trigger.Chance)
}

// weaponCritSkill casts the on-critical skill of attackerObj's weapon on
// targetObj: it activates on its chance, must land on the target, and then
// replaces the target's current effect of that skill with its own effects.
// No handler runs and nothing is broadcast.
func (p *ChanceProcs) weaponCritSkill(attackerObj, targetObj any) {
	caster, trigger, def, ok := p.weaponSkill(attackerObj, func(w *item.WeaponDetail) *item.SkillTrigger { return w.OnCritSkill })
	if !ok {
		return
	}
	target, ok := targetObj.(handlerskill.Actor)
	if !ok || !weaponChance(caster, trigger) {
		return
	}
	shield, landed := handlerskill.WeaponSkillLands(caster, target, def)
	if !landed {
		return
	}
	p.deliver(caster, true, func(handlerskill.MessageSink) EffectResult {
		return handlerEffectResult(handlerskill.LandCritSkill(caster, target, def, shield))
	})
}

// weaponMagicSkill casts the on-magic skill of casterObj's weapon on
// target as castDef lands there. Only a skill of the same offensive kind as
// castDef sets it off, never a toggle or a potion; it activates on its
// chance and, when offensive, must land on the target. A player caster is
// told the skill activated, then the skill's handler runs on the target.
func (p *ChanceProcs) weaponMagicSkill(casterObj any, target skilltarget.Actor, castDef modelskill.Definition) {
	caster, trigger, def, ok := p.weaponSkill(casterObj, func(w *item.WeaponDetail) *item.SkillTrigger { return w.OnCastSkill })
	if !ok || castDef.Offensive != def.Offensive {
		return
	}
	if castDef.Activation == modelskill.ActivationToggle || castDef.Potion {
		return
	}
	if !weaponChance(caster, trigger) {
		return
	}
	if def.Offensive {
		if _, landed := handlerskill.WeaponSkillLands(caster, target, def); !landed {
			return
		}
	}
	if caster.Kind() == actor.KindPlayer {
		p.deliver(caster, true, func(handlerskill.MessageSink) EffectResult {
			return EffectResult{Messages: []any{WeaponSkillActivated{Skill: def}}}
		})
	}
	if p.Skills == nil {
		return
	}
	p.deliver(caster, true, func(sink handlerskill.MessageSink) EffectResult {
		result, ok := p.Skills.UseResult(handlerskill.Cast{Caster: caster, Skill: def, Targets: []handlerskill.Actor{target}, Sink: sink})
		if !ok {
			return EffectResult{}
		}
		return handlerEffectResult(result)
	})
}

// fire runs owner's procs whose trigger event is in events, each against
// target. ownerHit reports that owner is the attacker or caster whose
// hit set the procs off.
func (p *ChanceProcs) fire(ownerObj any, ownerHit bool, targetObj any, events chanceEvents) {
	if events == 0 {
		return
	}
	owner, ok := ownerObj.(chanceOwner)
	if !ok || owner.Dead() {
		return
	}
	target, ok := targetObj.(skilltarget.Actor)
	if !ok {
		return
	}
	for _, e := range owner.EffectList().ChanceTriggers() {
		cond, ok := e.ChanceCondition()
		if ok && p.rolls(owner, cond, events) {
			p.castEffectTrigger(owner, ownerHit, e, target)
		}
	}
	for _, def := range p.chanceSkills(owner) {
		cond, ok, err := modelskill.ParseChanceCondition(def.ChanceType, def.ActivationChance)
		if ok && err == nil && p.rolls(owner, cond, events) {
			p.castChanceSkill(owner, ownerHit, def, target)
		}
	}
}

// rolls reports whether cond activates for events: its event is raised and,
// when it has an activation chance, owner wins the roll. The roll is drawn
// only for a raised event.
func (p *ChanceProcs) rolls(owner chanceOwner, cond modelskill.ChanceCondition, events chanceEvents) bool {
	return events.has(cond.Trigger) && (cond.Chance < 0 || owner.Roll(100) < cond.Chance)
}

// chanceSkills returns owner's known passive chance skills, by skill id.
func (p *ChanceProcs) chanceSkills(owner chanceOwner) []modelskill.Definition {
	holder, ok := owner.(chanceSkillHolder)
	if !ok || p.Definitions == nil {
		return nil
	}
	var out []modelskill.Definition
	for id, level := range holder.SkillLevels() {
		def, ok := p.Definitions.Definition(modelskill.Ref{ID: modelskill.ID(id), Level: level})
		if ok && def.Activation == modelskill.ActivationPassive && def.ChanceType != "" {
			out = append(out, def)
		}
	}
	slices.SortFunc(out, func(a, b modelskill.Definition) int { return cmp.Compare(a.ID, b.ID) })
	return out
}

// heldItems is a creature that reports the weapon and shield it holds. One
// that does not holds neither.
type heldItems interface {
	HeldItemTypeMask() int32
}

// castChanceSkill casts owner's passive chance skill def, or the skill it
// names to trigger, on target. def must first allow the weapon and shield
// owner holds, then pass its <cond> clauses.
func (p *ChanceProcs) castChanceSkill(owner chanceOwner, ownHit bool, def modelskill.Definition, target skilltarget.Actor) {
	var held int32
	if h, ok := owner.(heldItems); ok {
		held = h.HeldItemTypeMask()
	}
	if !WeaponAllowed(def, held) {
		p.deliver(owner, ownHit, func(handlerskill.MessageSink) EffectResult {
			return EffectResult{Messages: []any{ChanceWeaponNotAllowed{Skill: def}}}
		})
		return
	}
	if caster, ok := owner.(conditions.Source); ok {
		if clause, ok := conditions.EvaluateSkill(def, caster, target); !ok {
			p.deliver(owner, ownHit, func(handlerskill.MessageSink) EffectResult {
				return EffectResult{Messages: []any{ChanceConditionFailed{Skill: def, Clause: clause}}}
			})
			return
		}
	}
	if def.TriggeredID > 0 {
		triggered, ok := p.definition(def.TriggeredID, def.TriggeredLevel)
		if !ok || triggered.SkillType == "NOTDONE" {
			return
		}
		def = triggered
	}
	p.cast(owner, owner, ownHit, def, target)
}

// castEffectTrigger casts the skill chance-skill-trigger effect e names on
// target. A self-targeted skill is cast by owner, any other by the effect's
// effector, which is the hit's attacker or caster only when it is owner
// itself.
func (p *ChanceProcs) castEffectTrigger(owner chanceOwner, ownerHit bool, e *effect.Effect, target skilltarget.Actor) {
	if e.Template.TriggeredID <= 1 {
		return
	}
	def, ok := p.definition(e.Template.TriggeredID, e.Template.TriggeredLevel)
	if !ok || def.SkillType == "NOTDONE" {
		return
	}
	var caster chanceCaster = owner
	if def.Target != modelskill.TargetSelf {
		effector, ok := e.Effector.(chanceCaster)
		if !ok {
			return
		}
		caster = effector
	}
	p.cast(owner, caster, ownerHit && caster.ObjectID() == owner.ObjectID(), def, target)
}

// cast fires def from caster at the targets it resolves from owner and
// target. The skill's reuse starts even when it finds no target. ownHit
// reports that caster is the attacker or caster whose hit set it off.
func (p *ChanceProcs) cast(owner chanceOwner, caster chanceCaster, ownHit bool, def modelskill.Definition, target skilltarget.Actor) {
	key := ReuseKey(def)
	if caster.SkillDisabled(key) {
		return
	}
	if def.ReuseDelay > 0 {
		caster.DisableSkill(key, time.Duration(def.ReuseDelay)*time.Millisecond)
	}
	if p.Targets == nil || p.Skills == nil {
		return
	}
	handler, ok := p.Targets.Handler(def.Target)
	if !ok {
		return
	}
	affected := slices.DeleteFunc(handler.Targets(owner, target, &def), func(a skilltarget.Actor) bool { return a == nil })
	if len(affected) == 0 {
		return
	}
	ids := make([]int32, len(affected))
	castTargets := make([]handlerskill.Actor, len(affected))
	for i, a := range affected {
		ids[i] = a.ObjectID()
		castTargets[i] = a
	}
	owner.BroadcastSkillLaunched(int32(def.ID), int32(def.Level), ids)
	x, y, z := affected[0].Position()
	owner.BroadcastSkillUse(ids[0], x, y, z, int32(def.ID), int32(def.Level), 0, 0)
	p.deliver(caster, ownHit, func(sink handlerskill.MessageSink) EffectResult {
		result, ok := p.Skills.UseResult(handlerskill.Cast{Caster: caster, Skill: def, Targets: castTargets, Sink: sink})
		if !ok {
			return EffectResult{}
		}
		return handlerEffectResult(result)
	})
}

func (p *ChanceProcs) deliver(caster handlerskill.Creature, ownHit bool, apply func(handlerskill.MessageSink) EffectResult) {
	if p.Deliver == nil {
		apply(nil)
		return
	}
	p.Deliver(caster, ownHit, apply)
}

func (p *ChanceProcs) definition(id, level int) (modelskill.Definition, bool) {
	if p.Definitions == nil {
		return modelskill.Definition{}, false
	}
	return p.Definitions.Definition(modelskill.Ref{ID: modelskill.ID(id), Level: level})
}

func handlerEffectResult(result handlerskill.Result) EffectResult {
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
