package cast

import (
	"time"

	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// CastEffects owns the target-handler and skill-handler registries a cast
// resolves its affected targets and applies its effects through, once its
// Hit phase has consumed its final resource costs. It is the same plumbing
// the live player cast pipeline already drives, generalized so any caster
// satisfying skilltarget.Creature can reuse it.
type CastEffects struct {
	Targets *skilltarget.Registry
	Skills  *handlerskill.Registry
}

// Apply resolves def's affected target set from selected (the already
// cast-validated single selection) and applies def's effects to it. It
// mirrors the target/skill-handler dispatch step of the oracle's skill
// resolution, not the aggro, quest-event or PvP-flag side effects that land
// with the systems that implement them. A caster or selected target that
// doesn't satisfy the target-resolution surface is skipped rather than
// failing the cast, the same graceful degradation the player cast handler
// already uses for actor state this port hasn't modeled yet.
func (e CastEffects) Apply(caster skilltarget.Creature, selected skilltarget.Creature, def modelskill.Definition) {
	if e.Targets == nil || e.Skills == nil || caster == nil {
		return
	}

	handler, ok := e.Targets.Handler(def.Target)
	if !ok || !handler.CanCast(caster, selected, &def, false) {
		return
	}

	affected := handler.Targets(caster, selected, &def)
	if len(affected) == 0 {
		return
	}
	castTargets := make([]any, len(affected))
	for i, t := range affected {
		castTargets[i] = t
	}

	e.Skills.Use(handlerskill.Cast{
		Caster:  caster,
		Skill:   def,
		Targets: castTargets,
	})
}

// AIController bridges an AI intention-queue cast request to the same
// Controller state machine and CastEffects plumbing a live player cast
// drives, so the "ai" package's Attackable loop never needs to know about
// skill definitions, target handlers or resource costs itself. It satisfies
// the ai package's CastController interface structurally, without either
// package importing the other.
type AIController struct {
	Controller  *Controller
	Definitions Definitions
	Effects     CastEffects
	// Caster is the actor casting the skill, used both to start the cast
	// and, when it also satisfies skilltarget.Creature, as CastEffects'
	// caster.
	Caster Target
}

// Disabled reports whether the actor cannot attempt a new cast right now:
// already mid-cast, or (when Controller's actor optionally exposes it) every
// skill disabled.
func (a *AIController) Disabled() bool {
	if a.Controller == nil {
		return true
	}
	if a.Controller.CastingNow() {
		return true
	}
	if d, ok := a.Controller.actor.(interface{ AllSkillsDisabled() bool }); ok {
		return d.AllSkillsDisabled()
	}
	return false
}

// Range returns ref's cast range, used to decide whether the actor must
// close distance on the target before attempting the cast.
func (a *AIController) Range(ref modelskill.Ref) int {
	def, ok := a.definition(ref)
	if !ok {
		return 0
	}
	return def.CastRange
}

// StopsMovement reports whether ref's cast animation is long enough that the
// actor should stop moving and face its target before the final cast
// attempt, mirroring the oracle's hit-time threshold.
func (a *AIController) StopsMovement(ref modelskill.Ref) bool {
	def, ok := a.definition(ref)
	return ok && def.HitTime > 50
}

// CanAttempt validates the lightweight pre-movement cast gate (reuse
// cooldown) for ref, before the actor commits to closing distance on
// target.
func (a *AIController) CanAttempt(target attackable.Combatant, ref modelskill.Ref) bool {
	if a.Controller == nil || target == nil {
		return false
	}
	def, ok := a.definition(ref)
	if !ok {
		return false
	}
	return !a.Controller.SkillOnCooldown(def)
}

// CanCast validates the final HP/MP/mute/reuse/item gates immediately before
// the cast commits.
func (a *AIController) CanCast(target attackable.Combatant, ref modelskill.Ref) bool {
	if a.Controller == nil || target == nil {
		return false
	}
	def, ok := a.definition(ref)
	if !ok {
		return false
	}
	castTarget, ok := any(target).(Target)
	if !ok {
		return false
	}
	return a.Controller.CanCast(castTarget, def) == nil
}

// Cast starts the cast against target and schedules its Launch, Hit and
// Finish phases, applying def's effects through Effects once the Hit phase
// consumes its final resource cost.
func (a *AIController) Cast(target attackable.Combatant, ref modelskill.Ref) {
	if a.Controller == nil || target == nil {
		return
	}
	def, ok := a.definition(ref)
	if !ok {
		return
	}
	castTarget, ok := any(target).(Target)
	if !ok {
		return
	}

	plan, err := a.Controller.Start(time.Now(), castTarget, def)
	if err != nil {
		return
	}

	caster, _ := a.Caster.(skilltarget.Creature)
	selected, _ := any(target).(skilltarget.Creature)
	a.Controller.Schedule(plan, Hooks{
		Hit: func() {
			a.Effects.Apply(caster, selected, def)
		},
	})
}

func (a *AIController) definition(ref modelskill.Ref) (modelskill.Definition, bool) {
	if a.Definitions == nil {
		return modelskill.Definition{}, false
	}
	return a.Definitions.Definition(ref)
}
