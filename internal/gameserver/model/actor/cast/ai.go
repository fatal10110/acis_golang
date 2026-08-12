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
	// OnLaunchAbort sends the caster-visible result of a launch-phase gate
	// failure. Network wiring owns the system-message encoding.
	OnLaunchAbort func(LaunchAbortReason)
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

// SkillType returns ref's raw skillType tag.
func (a *AIController) SkillType(ref modelskill.Ref) string {
	def, ok := a.definition(ref)
	if !ok {
		return ""
	}
	return def.SkillType
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

// magicCastBroadcaster is the observer-broadcast surface an AI-initiated
// cast's caster optionally exposes, mirroring the reference sequence in
// CreatureCast.java (the same doCast/onMagicLaunch path PlayerCast chains
// into via super.doCast, so player and AI casts share it): MagicSkillUse
// broadcasts at cast start with the computed hitTime/reuseDelay
// (CreatureCast.java:148), and MagicSkillLaunched broadcasts at the launch
// timer — hitTime-400ms — with the full launch-resolved target list
// (CreatureCast.java:165,232-234). A caster that doesn't implement it (e.g.
// in tests) simply broadcasts nothing.
type magicCastBroadcaster interface {
	BroadcastSkillUse(targetID int32, targetX, targetY, targetZ int, skillID, level int32, hitTime, reuseDelay int) error
	BroadcastSkillLaunched(skillID, level int32, targetIDs []int32) error
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

	broadcaster, _ := a.Caster.(magicCastBroadcaster)

	// MagicSkillUse broadcasts the instant the cast starts, matching
	// CreatureCast.doCast's broadcastPacket call before the launch timer is
	// even scheduled (CreatureCast.java:148,165).
	if broadcaster != nil {
		tx, ty, tz := castTarget.Position()
		if err := broadcaster.BroadcastSkillUse(castTarget.ObjectID(), tx, ty, tz, int32(def.ID), int32(def.Level),
			int(plan.HitTime/time.Millisecond), int(plan.ReuseDelay/time.Millisecond)); err != nil {
			a.Controller.log.Warn().Err(err).Msg("cast: skill-use broadcast")
		}
	}

	a.Controller.Schedule(plan, Hooks{
		Launch: func() bool {
			if reason := RevalidateLaunch(a.Caster, castTarget, def); reason != LaunchAbortNone {
				if a.OnLaunchAbort != nil && reason != LaunchAbortTargetLost {
					a.OnLaunchAbort(reason)
				}
				return false
			}
			if broadcaster != nil {
				// The reference recomputes _targets = getTargetList(...) at
				// the launch timer and broadcasts that full set
				// (CreatureCast.java:232-234), not just the preselected
				// single target.
				targetIDs := ResolveTargetIDs(a.Effects, a.Caster, castTarget, def)
				if targetIDs == nil {
					targetIDs = []int32{castTarget.ObjectID()}
				}
				if err := broadcaster.BroadcastSkillLaunched(int32(def.ID), int32(def.Level), targetIDs); err != nil {
					a.Controller.log.Warn().Err(err).Msg("cast: skill-launched broadcast")
				}
			}
			return true
		},
		Hit: func() {
			// FUSION is dispatched to PlayerCast.doFusionCast only for
			// player casters (PlayerAI.java:300-301); CreatureCast's
			// override is an empty stub — "Non-Player Creatures cannot use
			// FUSION or SIGNETS" (CreatureCast.java:81-84). AIController
			// drives every non-player-initiated cast, so it must skip
			// FUSION here rather than let it reach fusionHandler, which
			// has no caster-type gate of its own.
			if def.SkillType == "FUSION" {
				return
			}
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
