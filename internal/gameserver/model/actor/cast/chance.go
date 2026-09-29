package cast

import (
	"cmp"
	"slices"
	"time"

	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
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
	// delivers one. Nil runs apply and drops the result.
	Deliver func(caster handlerskill.Creature, apply func() EffectResult)
}

// ChanceConditionFailed is a triggered cast refused by one of its skill's
// <cond> clauses. Its message goes to the owner, as a failed cast's does.
type ChanceConditionFailed struct {
	Skill  modelskill.Definition
	Clause modelskill.ConditionClause
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

// AttackHit runs the procs of a physical hit that dealt damage: the
// attacker's on-hit procs (and on-crit ones for a critical hit) against
// target, then target's on-attacked procs against the attacker. Both run
// on the attacker's queue, in the hit's own call, like the hit itself.
func (p *ChanceProcs) AttackHit(attacker, target any, crit bool) {
	if p == nil {
		return
	}
	hit := eventsOf(modelskill.TriggerOnHit)
	if crit {
		hit |= eventsOf(modelskill.TriggerOnCrit)
	}
	p.fire(attacker, target, hit)
	p.fire(target, attacker, eventsOf(modelskill.TriggerOnAttacked, modelskill.TriggerOnAttackedHit))
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
		p.fire(caster, target, cast)
		p.fire(target, caster, taken)
	}
}

// fire runs owner's procs whose trigger event is in events, each against
// target.
func (p *ChanceProcs) fire(ownerObj, targetObj any, events chanceEvents) {
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
			p.castEffectTrigger(owner, e, target)
		}
	}
	for _, def := range p.chanceSkills(owner) {
		cond, ok, err := modelskill.ParseChanceCondition(def.ChanceType, def.ActivationChance)
		if ok && err == nil && p.rolls(owner, cond, events) {
			p.castChanceSkill(owner, def, target)
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

// castChanceSkill casts owner's passive chance skill def, or the skill it
// names to trigger, on target.
func (p *ChanceProcs) castChanceSkill(owner chanceOwner, def modelskill.Definition, target skilltarget.Actor) {
	if caster, ok := owner.(conditions.Source); ok {
		if clause, ok := conditions.EvaluateSkill(def, caster, target); !ok {
			p.deliver(owner, func() EffectResult {
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
	p.cast(owner, owner, def, target)
}

// castEffectTrigger casts the skill chance-skill-trigger effect e names on
// target. A self-targeted skill is cast by owner, any other by the effect's
// effector.
func (p *ChanceProcs) castEffectTrigger(owner chanceOwner, e *effect.Effect, target skilltarget.Actor) {
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
	p.cast(owner, caster, def, target)
}

// cast fires def from caster at the targets it resolves from owner and
// target. The skill's reuse starts even when it finds no target.
func (p *ChanceProcs) cast(owner chanceOwner, caster chanceCaster, def modelskill.Definition, target skilltarget.Actor) {
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
	p.deliver(caster, func() EffectResult {
		result, ok := p.Skills.UseResult(handlerskill.Cast{Caster: caster, Skill: def, Targets: castTargets})
		if !ok {
			return EffectResult{}
		}
		return handlerEffectResult(result)
	})
}

func (p *ChanceProcs) deliver(caster handlerskill.Creature, apply func() EffectResult) {
	if p.Deliver == nil {
		apply()
		return
	}
	p.Deliver(caster, apply)
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
