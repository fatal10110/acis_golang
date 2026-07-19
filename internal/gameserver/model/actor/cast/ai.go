package cast

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// AIController bridges an AI intention-queue cast request to the same
// Controller state machine and ApplyEffects plumbing a live player cast
// drives, so the "ai" package's Attackable loop never needs to know about
// skill definitions, target handlers or resource costs itself. It satisfies
// the ai package's CastController interface structurally, without either
// package importing the other.
type AIController struct {
	Controller  *Controller
	Definitions Definitions
	Effects     EffectHandlers
	// Caster is the actor casting the skill, used both to start the cast
	// and, when it also satisfies skilltarget.Creature, as ApplyEffects'
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

	a.Controller.Schedule(plan, Hooks{
		Hit: func() {
			ApplyEffects(a.Effects, a.Caster, castTarget, def)
		},
	})
}

func (a *AIController) definition(ref modelskill.Ref) (modelskill.Definition, bool) {
	if a.Definitions == nil {
		return modelskill.Definition{}, false
	}
	return a.Definitions.Definition(ref)
}
