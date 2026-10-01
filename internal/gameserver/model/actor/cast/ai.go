package cast

import (
	"time"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
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
	// and as ApplyEffects' caster.
	Caster AICaster
	// OnLaunchAbort sends the caster-visible result of a launch-phase gate
	// failure. Network wiring owns the system-message encoding.
	OnLaunchAbort func(LaunchAbortReason)
	// OnTargetRejection sends the caster-visible result of a failed target
	// condition (CanCastPlayable's last gate). Network wiring owns the
	// system-message encoding.
	OnTargetRejection func(skilltarget.CastRejection, modelskill.Definition)
	// OnCastRefusal sends the caster-visible reason of a playable cast
	// refused by AttemptCast or CanCastPlayable: the error names the failed
	// gate (ErrSkillDisabled, ErrNotEnoughMP, ErrCantSeeTarget, a
	// *ConditionError, ...). Network wiring owns the system-message
	// encoding.
	OnCastRefusal func(error, modelskill.Definition)
	// OnHitResult receives the EffectResult of a resolved Hit-phase cast.
	// Summon casters wire it to forward the result to the owner, as a
	// summon forwards its packets. Hostile NPC casters wire it to a
	// nil-live-safe delivery hook so target-addressed messages (MagicResist,
	// ManaDrain) still reach a real online target even though
	// caster-addressed ones go nowhere for an NPC caster (issue #2350).
	// Each handler message reaches it as its own
	// one-message result the moment the handler produces it, so the message
	// keeps its place among the frames the hit itself sends; the result
	// that follows the hit carries the rest.
	OnHitResult func(EffectResult)
	// HitNeedsPresence drops the hit when the caster has left the world
	// since launch. Set it only for casters whose every removal aborts the
	// cast first, so the drop never fires where a removed caster would
	// still land its skill.
	HitNeedsPresence bool
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
	return a.Controller.actor.AllSkillsDisabled()
}

func (a *AIController) CastingNow() bool {
	return a.Controller != nil && a.Controller.CastingNow()
}

// Stop aborts an in-flight AI cast.
func (a *AIController) Stop() {
	if a.Controller != nil {
		a.Controller.Stop()
	}
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
// attempt, against the hit-time threshold.
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

// CanDesire runs the gates an AI cast request for ref passes before it is
// queued: the skill's reuse, then the MP and the HP its hit takes.
func (a *AIController) CanDesire(target attackable.Combatant, ref modelskill.Ref) bool {
	if !a.CanAttempt(target, ref) {
		return false
	}
	def, _ := a.definition(ref)
	actor := a.Controller.actor
	if mp := actor.MPCost(def); mp > 0 && mp > actor.MP() {
		return false
	}
	return def.HPConsume <= 0 || def.HPConsume <= actor.HP()
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

// FinalTarget resolves the creature ref's target type aims at, given the
// commanded target (nil when there is none): the caster itself for its
// self-centered types, its owner for OWNER_PET, the commanded target for ONE.
// nil means the skill has no final target, and the request is dropped.
func (a *AIController) FinalTarget(target attackable.Combatant, ref modelskill.Ref) attackable.Combatant {
	def, ok := a.definition(ref)
	if !ok || a.Caster == nil || a.Effects.Targets == nil {
		return nil
	}
	handler, ok := a.Effects.Targets.Handler(def.Target)
	if !ok {
		return nil
	}
	selected, _ := any(target).(skilltarget.Actor)
	final, _ := handler.FinalTarget(a.Caster, selected, &def).(attackable.Combatant)
	return final
}

// AttemptCast is CanAttempt for a playable caster's cast request: a skill
// still cooling down is reported through OnCastRefusal.
func (a *AIController) AttemptCast(target attackable.Combatant, ref modelskill.Ref) bool {
	if a.CanAttempt(target, ref) {
		return true
	}
	if def, ok := a.definition(ref); ok && a.Controller != nil && target != nil {
		a.refuse(ErrSkillDisabled, def)
	}
	return false
}

// CanCastPlayable runs a playable caster's gates immediately before its cast
// commits, in the specified order: HP/MP and mute, line of sight to the
// target of a ranged skill, the skill's own conditions, the Olympiad skill
// ban and item cost (CanCastSighted), and last the target conditions judged
// with ctrl. The first failure is reported through OnCastRefusal, or
// OnTargetRejection for a target condition.
func (a *AIController) CanCastPlayable(target attackable.Combatant, ref modelskill.Ref, ctrl bool) bool {
	if a.Controller == nil || a.Caster == nil || target == nil {
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
	if err := a.Controller.CanCastSighted(a.Caster, castTarget, def); err != nil {
		a.refuse(err, def)
		return false
	}
	return a.meetsCastConditions(target, def, ctrl)
}

func (a *AIController) refuse(err error, def modelskill.Definition) {
	if a.OnCastRefusal != nil {
		a.OnCastRefusal(err, def)
	}
}

// meetsCastConditions applies def's target-type conditions for a playable
// caster against target, the last check before a playable's cast commits.
// A failure is reported through OnTargetRejection.
func (a *AIController) meetsCastConditions(target attackable.Combatant, def modelskill.Definition, ctrl bool) bool {
	aimed, ok := any(target).(skilltarget.Actor)
	if !ok || a.Caster == nil {
		return false
	}
	rejection := skilltarget.CastRejectionFor(def.Target, a.Caster, aimed, &def, ctrl)
	if rejection == skilltarget.CastRejectNone {
		return true
	}
	if a.OnTargetRejection != nil {
		a.OnTargetRejection(rejection, def)
	}
	return false
}

// MeetsHPMPDisabled reports whether the actor currently has the HP/MP and is
// not muted for ref against target.
func (a *AIController) MeetsHPMPDisabled(target attackable.Combatant, ref modelskill.Ref) bool {
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
	return a.Controller.MeetsHPMPDisabled(castTarget, def) == nil
}

// AICaster is an AI-driven caster: the launch-revalidated creature plus the
// observer broadcasts of its cast, in the sequence player and AI casts
// share: MagicSkillUse broadcasts at cast start with the computed
// hitTime/reuseDelay, MagicSkillLaunched broadcasts at the launch timer —
// hitTime-400ms — with the full launch-resolved target list, and
// MagicSkillCanceled broadcasts whenever an in-flight cast aborts (only
// while a cast is in flight, for an NPC as for any creature). Casters without
// observers to notify report nil.
type AICaster interface {
	LaunchCaster
	BroadcastSkillUse(targetID int32, targetX, targetY, targetZ int, skillID, level int32, hitTime, reuseDelay int)
	BroadcastSkillLaunched(skillID, level int32, targetIDs []int32)
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
	castTarget := Target(target)

	plan, err := a.Controller.Start(a.Controller.Now(), castTarget, def)
	if err != nil {
		return
	}

	// MagicSkillUse broadcasts the instant the cast starts, before the
	// launch timer is even scheduled.
	tx, ty, tz := castTarget.Position()
	a.Caster.BroadcastSkillUse(castTarget.ObjectID(), tx, ty, tz, int32(def.ID), int32(def.Level),
		int(plan.HitTime/time.Millisecond), int(plan.ReuseDelay/time.Millisecond))

	// launchTargets is resolved once, in the Launch hook, and reused
	// unchanged by Hit: the launch assigns the target list once and the
	// hit timer's skill call reads it again rather than re-deriving it.
	// That keeps the MagicSkillLaunched broadcast and the
	// effect-affected set as one snapshot instead of two independent
	// resolutions 400ms apart.
	var launchTargets []skilltarget.Actor
	var launchResolved bool

	a.Controller.Schedule(plan, Hooks{
		Launch: func() bool {
			if reason := RevalidateLaunch(a.Caster, castTarget, def); reason != LaunchAbortNone {
				if a.OnLaunchAbort != nil && reason != LaunchAbortTargetLost {
					a.OnLaunchAbort(reason)
				}
				return false
			}
			launchTargets, launchResolved = ResolveAffected(a.Effects, a.Caster, castTarget, def)
			a.Controller.SetLaunchTargets(len(launchTargets))
			// The target list is recomputed at the launch timer and that
			// full set is broadcast; when resolution finds no affected
			// targets, the empty list is broadcast as-is
			// (no skip, no synthesized fallback target) — the wire
			// builder already writes that form (0,0) unconditionally.
			targetIDs := make([]int32, len(launchTargets))
			for i, t := range launchTargets {
				targetIDs[i] = t.ObjectID()
			}
			a.Caster.BroadcastSkillLaunched(int32(def.ID), int32(def.Level), targetIDs)
			return true
		},
		Hit: func() {
			// FUSION is dispatched to the fusion cast path only for player
			// casters; for any other creature that path is an empty stub —
			// non-player creatures cannot use FUSION or SIGNETS. AIController
			// drives every non-player-initiated cast, so it must skip
			// FUSION here rather than let it reach fusionHandler, which
			// has no caster-type gate of its own.
			if def.SkillType == "FUSION" || !launchResolved {
				return
			}
			// Off the grid, an object knows nothing, itself included.
			if a.HitNeedsPresence && !a.Caster.Knows(a.Caster) {
				return
			}
			handlers := a.Effects
			if a.OnHitResult != nil && handlers.Sink == nil {
				handlers.Sink = func(message any) {
					a.OnHitResult(EffectResult{Messages: []any{message}})
				}
			}
			result := ApplyResolvedEffectsResult(handlers, a.Caster, launchTargets, def)
			if a.OnHitResult != nil {
				a.OnHitResult(result)
			}
		},
	})
}

func (a *AIController) definition(ref modelskill.Ref) (modelskill.Definition, bool) {
	if a.Definitions == nil {
		return modelskill.Definition{}, false
	}
	return a.Definitions.Definition(ref)
}
