package network

import (
	"errors"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	itemhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/item"
	skillhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func (l *GameClientLink) handleMagicSkillUse(live *livePlayer, req clientpackets.RequestMagicSkillUse) {
	if live == nil {
		sendMagicActionFailed(live)
		return
	}

	selected := live.Target()
	def, known := l.skills.Definition(modelskill.Ref{ID: modelskill.ID(req.SkillID), Level: live.SkillLevel(int(req.SkillID))})
	castable := known && (def.Activation == modelskill.ActivationActive || def.Activation == modelskill.ActivationToggle)
	if known {
		if itemhandler.RecallCastBlockedByKarma(def.SkillType, live.Karma(), l.playerConfig.KarmaPlayerCanTeleport) {
			sendMagicActionFailed(live)
			return
		}
		// A mounted player can switch no toggle on or off: the request is
		// refused before it becomes a cast intention, so nothing is paid,
		// queued or broadcast.
		if def.Activation == modelskill.ActivationToggle && live.Mounted() {
			sendMagicActionFailed(live)
			return
		}
		if castable && !l.attemptMagicSkill(live, def, selected) {
			return
		}
	}
	// A request that passed the pre-attempt gate while a swing or another
	// cast is in flight, toggle or not, becomes the next CAST intention and
	// is answered with ActionFailed; the swing's or cast's end runs it.
	// Starting it now would pay its costs and broadcast MagicSkillUse (or
	// switch a toggle) before what is in flight has finished. It replaces an
	// attack queued behind the same cast, and the attack the swing in
	// flight is for. itemAICastBusy is the wait predicate every cast request
	// shares, the sit-down and stand-up transitions included.
	//
	// A pets-row read still in flight stands in for a cast the caster is
	// still in, whether or not its hold already ended, so a servitor request
	// made across it waits for the pet to land the same way, and the summon
	// slot gate then refuses it before any cost.
	if castable && (itemAICastBusy(live) || l.restoringServitor(live, def)) {
		live.deferMagicSkill(req, selected)
		if live.combat != nil {
			live.combat.ReplaceWithCast()
		}
		sendMagicActionFailed(live)
		return
	}
	l.castMagicSkill(live, req, def, known, selected)
}

// castMagicSkill starts a skill request that has cleared its request-time
// gates, fresh or resumed, against selected.
func (l *GameClientLink) castMagicSkill(live *livePlayer, req clientpackets.RequestMagicSkillUse, def modelskill.Definition, known bool, selected world.Tracked) {
	// The request is the CAST intention now, whatever its outcome: the attack
	// intention it replaced swings again only if the cast ends with
	// nextActionAttack.
	if known && (def.Activation == modelskill.ActivationActive || def.Activation == modelskill.ActivationToggle) {
		if live.combat != nil {
			live.combat.ReplaceWithCast()
		}
		live.endFollow()
	}
	live.Character.SetCastModifiers(req.CtrlPressed, req.ShiftPressed)
	if known && (def.Activation == modelskill.ActivationActive || def.Activation == modelskill.ActivationToggle) {
		// A walk or queued action parked for an earlier intention is
		// replaced by this CAST intention, whether it casts now or walks.
		live.clearParkedApproaches()
		if def.Target != modelskill.TargetGround {
			target := l.magicSkillFinalTarget(live, def, selected)
			if l.walkToCastTarget(live, target, def.CastRange, req.ShiftPressed, func() { live.approachMagicSkill(req, selected, target.ObjectID()) }) {
				return
			}
		}
	}
	if known && def.Activation == modelskill.ActivationToggle {
		l.handleToggleSkillUse(live, req, selected)
		return
	}
	controller := l.castController(live)
	// The pre-attempt gate above ran before this GROUND approach walk, so a
	// recast still on cooldown never starts walking toward the signet.
	if def.Target == modelskill.TargetGround && l.walkToGroundCast(live, req, selected, def.CastRange) {
		return
	}

	var afterCanCast func() error
	if known && def.Target == modelskill.TargetGround {
		afterCanCast = l.groundCastAfterCanCast(live, def)
	}
	started, err := actorcast.StartPlayerSkill(actorcast.PlayerSkillRequest{
		Controller:  controller,
		Caster:      live.Character,
		Selected:    selected,
		SkillID:     int(req.SkillID),
		Definitions: l.skills,
		Ctrl:        req.CtrlPressed,
		Shift:       req.ShiftPressed,
		Hooks: actorcast.StartHooks{
			ResolveTarget: l.resolveMagicSkillTarget,
			StopMovement:  l.stopMovementForCast(live),
			AfterCanCast:  afterCanCast,
		},
	})
	if err != nil {
		if errors.Is(err, errGroundCastRejected) {
			return
		}
		// A nextActionAttack skill refused at its cost and condition checks
		// still hands on to the attack, after the refusal's own packets.
		if started.CanCastFailure {
			defer live.attackAfterCast(started.Definition, castCombatant(started.Target), req.ShiftPressed, false)
		}
		// A player's cast that fails its cost or target conditions after the
		// hit-time stop answers with its reason alone: no ActionFailed, no
		// heading toward the target and no MoveToPawn (those belong to the
		// pet and NPC cast paths). A locked door is refused with no packet
		// at all: the target check returns silently.
		if started.CanCastFailure && magicCastFailureReasonOnly(err) {
			sendMagicCastFailureReason(live, started.Definition, err)
			return
		}
		if errors.Is(err, actorcast.ErrInvalidTarget) && started.Rejection != skilltarget.CastRejectNone {
			sendTargetCastRejection(live, started.Rejection, started.Definition)
			return
		}
		if errors.Is(err, actorcast.ErrInvalidTarget) && started.Target == nil {
			// No final target: the request is dropped with ActionFailed
			// alone.
			sendMagicActionFailed(live)
			return
		}
		sendMagicCastFailure(live, started.Definition, err)
		return
	}
	def = started.Definition
	target := started.Target
	plan := started.Plan

	handlers := l.castEffects()
	switch def.SkillType {
	case "FUSION":
		l.startFusionCast(live, controller, target, def, plan)
		return
	case "SIGNET_CASTTIME":
		l.startSignetCast(live, controller, handlers, target, def, plan)
		return
	}

	l.broadcastCastStart(live, target, def, plan)
	if plan.GaugeDuration > 0 {
		live.SendFrame(serverpackets.FrameSetupGauge(serverpackets.GaugeBlue, millis(plan.GaugeDuration), millis(plan.GaugeDuration)))
	}
	sendSkillItemCharge(live, def, plan.ItemCharge)

	var affected []skilltarget.Actor
	controller.Schedule(plan, actorcast.Hooks{
		Launch: func() bool {
			var ok bool
			affected, ok = l.launchCastTargets(live, target, def)
			return ok
		},
		Hit: func() {
			l.applyCastHit(live, handlers, affected, def)
		},
		Failed: func(err error) {
			// Each cost the hit already paid sent its own status, so the
			// reason is all that is left before the abort frames.
			sendMagicCastFailureReason(live, def, err)
		},
	})
}

// castCombatant is a cast's final target as a creature, nil when it is not
// one.
func castCombatant(target actorcast.Target) attackable.Combatant {
	combatant, _ := target.(attackable.Combatant)
	return combatant
}

// broadcastCastStart sends a player's cast-start MagicSkillUse to everyone
// watching, then USE_S1 to the caster.
func (l *GameClientLink) broadcastCastStart(live *livePlayer, target actorcast.Target, def modelskill.Definition, plan actorcast.Plan) {
	casterObject := skillCastObject(live)
	targetObject := skillCastObject(target)
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameMagicSkillUse(
			casterObject,
			targetObject,
			int32(def.ID),
			int32(def.Level),
			millis(plan.HitTime),
			millis(plan.ReuseDelay),
			false,
		)
	})
	live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageUseS1, int32(def.ID), int32(def.Level)))
}

// startFusionCast lands a FUSION skill's force effect on its target when the
// channel opens, before the caster's MagicSkillUse, USE_S1 and gauge go out.
// The gauge is sent whatever the hit time: a channel has no short-cast
// cutoff.
func (l *GameClientLink) startFusionCast(live *livePlayer, controller *actorcast.Controller, target actorcast.Target, def modelskill.Definition, plan actorcast.Plan) {
	live.setFusionTarget(target.ObjectID())
	finishFusion := func() {
		// Only a creature carries the triggered fusion effect to decrease.
		if effected, ok := target.(attackable.Combatant); ok {
			skillhandler.DecreaseFusion(l.skills, live.Character, effected, def)
		}
		live.clearFusionTarget(target.ObjectID())
	}
	// The force lands on the target the cast committed to, its conditions
	// not judged again.
	if effected, ok := target.(attackable.Combatant); ok {
		skillhandler.StartFusion(l.skills, live.Character, effected, def)
	}

	l.broadcastCastStart(live, target, def, plan)
	live.SendFrame(serverpackets.FrameSetupGauge(serverpackets.GaugeBlue, millis(plan.HitTime), millis(plan.HitTime)))
	if !controller.ScheduleFusion(plan, time.Second, func() bool {
		return actorcast.FusionChannelValid(live.Character, target, def.CastRange)
	}, finishFusion) {
		finishFusion()
	}
}

// startSignetCast runs a SIGNET_CASTTIME cast on the fusion-cast timeline:
// its effect lands on the target when the cast starts, ahead of the
// caster's MagicSkillUse, USE_S1 and gauge, and the gauge is sent whatever
// the hit time. The launch broadcasts MagicSkillLaunched with no mid-cast
// revalidation, then the controller charges shots and the final MP, and the
// cast ends with no cool phase (Controller.ScheduleSignetCast).
func (l *GameClientLink) startSignetCast(live *livePlayer, controller *actorcast.Controller, handlers actorcast.EffectHandlers, target actorcast.Target, def modelskill.Definition, plan actorcast.Plan) {
	if resolved, ok := target.(skilltarget.Actor); ok {
		handlers.Sink = l.playerMessageSink(live, nil)
		result := actorcast.ApplyResolvedEffectsResult(handlers, live.Character, []skilltarget.Actor{resolved}, def)
		l.settlePvPChanges(live)
		l.syncCubicTargets(live, result, def)
	}

	l.broadcastCastStart(live, target, def, plan)
	live.SendFrame(serverpackets.FrameSetupGauge(serverpackets.GaugeBlue, millis(plan.HitTime), millis(plan.HitTime)))
	controller.ScheduleSignetCast(plan, func() int {
		affected, _ := l.broadcastLaunchTargets(live, target, def)
		return len(affected)
	}, func(err error) {
		sendMagicCastFailureReason(live, def, err)
	})
}

// launchCastTargets runs a player cast's launch: the mid-cast revalidation,
// then the one resolution of def's affected set, broadcast in
// MagicSkillLaunched and returned for the hit to reuse as is. A creature
// that dies or moves between launch and hit is left to each skill handler's
// own per-target checks. ok is false when the cast must stop.
func (l *GameClientLink) launchCastTargets(live *livePlayer, target actorcast.Target, def modelskill.Definition) (affected []skilltarget.Actor, ok bool) {
	if reason := actorcast.RevalidateLaunch(live.Character, target, def); reason != actorcast.LaunchAbortNone {
		sendLaunchAbort(live, reason)
		return nil, false
	}
	affected, ok = l.broadcastLaunchTargets(live, target, def)
	if ok {
		l.castController(live).SetLaunchTargets(len(affected))
	}
	return affected, ok
}

// broadcastLaunchTargets resolves def's affected set from target and
// broadcasts it in MagicSkillLaunched. ok is false when no set can be
// resolved.
func (l *GameClientLink) broadcastLaunchTargets(live *livePlayer, target actorcast.Target, def modelskill.Definition) (affected []skilltarget.Actor, ok bool) {
	handler, ok := l.targets.Handler(def.Target)
	if !ok {
		return nil, false
	}
	resolvedTarget, ok := target.(skilltarget.Actor)
	if !ok {
		return nil, false
	}
	affected = handler.Targets(live.Character, resolvedTarget, &def)
	targetIDs := make([]int32, 0, len(affected))
	for _, affectedTarget := range affected {
		targetIDs = append(targetIDs, affectedTarget.ObjectID())
	}
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameMagicSkillLaunched(live.ObjectID(), int32(def.ID), int32(def.Level), targetIDs)
	})
	return affected, true
}

// applyCastHit dispatches a player cast's effects to the affected set its
// launch resolved. Every summon in that set first republishes its status
// (its owner's pet window and its observers), before any effect lands. The
// final MP/HP costs already sent their own statuses, and every change the
// effects make to the caster's vitals reports its own status where it
// happens, so the hit sends none of its own. A PK kill the hit made, and
// the flag the skill raised after it, settle before anything else the hit
// sends.
func (l *GameClientLink) applyCastHit(live *livePlayer, handlers actorcast.EffectHandlers, affected []skilltarget.Actor, def modelskill.Definition) {
	l.applyItemCastHit(live, handlers, affected, def, nil)
}

// applyItemCastHit is applyCastHit for a cast that hands item to its skill
// handler.
func (l *GameClientLink) applyItemCastHit(live *livePlayer, handlers actorcast.EffectHandlers, affected []skilltarget.Actor, def modelskill.Definition, item any) {
	actorcast.RefreshSummonTargets(affected)
	handlers.Sink = l.playerMessageSink(live, nil)
	result := actorcast.ApplyResolvedItemEffectsResult(handlers, live.Character, affected, def, item)
	l.settlePvPChanges(live)
	l.syncCubicTargets(live, result, def)
}

// attemptMagicSkill runs a player's skill request through the pre-attempt
// gate ahead of the swing queue, the approach walk and the cast itself. A
// dead caster and a request with no final target get a bare ActionFailed;
// a gate failure gets its reason and ActionFailed. It reports whether the
// request may go on.
func (l *GameClientLink) attemptMagicSkill(live *livePlayer, def modelskill.Definition, selected world.Tracked) bool {
	if live.Character.Dead() {
		sendMagicActionFailed(live)
		return false
	}
	target := l.magicSkillFinalTarget(live, def, selected)
	if target == nil {
		sendMagicActionFailed(live)
		return false
	}
	if err := l.castController(live).CanPlayerAttemptCast(live.Character, target, def); err != nil {
		sendMagicCastFailure(live, def, err)
		return false
	}
	return true
}

// magicSkillFinalTarget is the creature def would be cast on given the
// caster's selection, before any cast condition is checked.
func (l *GameClientLink) magicSkillFinalTarget(live *livePlayer, def modelskill.Definition, selected world.Tracked) skilltarget.Actor {
	return l.skillFinalTarget(live, selected, def)
}

// magicCastFailureReasonOnly reports the cast-condition failures a player
// hears about only through their reason message.
func magicCastFailureReasonOnly(err error) bool {
	return errors.Is(err, actorcast.ErrNotEnoughMP) ||
		errors.Is(err, actorcast.ErrNotEnoughHP) ||
		errors.Is(err, actorcast.ErrMagicMuted) ||
		errors.Is(err, actorcast.ErrPhysicalMuted) ||
		errors.Is(err, actorcast.ErrNotEnoughItems) ||
		errors.Is(err, actorcast.ErrWeaponNotAllowed) ||
		errors.Is(err, actorcast.ErrCantSeeTarget) ||
		errors.Is(err, actorcast.ErrOlympiadSkill) ||
		errors.Is(err, actorcast.ErrSummonOnlyOne) ||
		errors.Is(err, actorcast.ErrSummonInCombat) ||
		errors.Is(err, actorcast.ErrSummonOnBoat) ||
		errors.As(err, new(*actorcast.ConditionError))
}

func (l *GameClientLink) stopMovementForCast(live *livePlayer) func() {
	return func() {
		if live == nil || live.move == nil {
			return
		}
		live.move.Stop()
	}
}

var errGroundCastRejected = errors.New("ground cast rejected")

func (l *GameClientLink) groundCastAfterCanCast(live *livePlayer, def modelskill.Definition) func() error {
	return func() error {
		x, y, z := live.GroundTarget()
		switch skilltarget.GroundCastFailureFor(live.Character, &def) {
		case skilltarget.GroundCastNoLineOfSight:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCantSeeTarget))
			sendMagicActionFailed(live)
			return errGroundCastRejected
		case skilltarget.GroundCastPeaceZone:
			live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageS1CannotBeUsed, int32(def.ID), int32(def.Level)))
			sendMagicActionFailed(live)
			return errGroundCastRejected
		}
		live.Character.SetHeading(live.CurrentLocation().HeadingTo(location.Location{X: x, Y: y, Z: z}))
		l.broadcastLiveFrame(live, func() wire.Frame {
			return serverpackets.FrameValidateLocation(live.ObjectID(), live.CurrentLocation(), live.CurrentHeading())
		})
		return nil
	}
}

func (l *GameClientLink) walkToGroundCast(live *livePlayer, req clientpackets.RequestMagicSkillUse, selected world.Tracked, castRange int) bool {
	x, y, z := live.GroundTarget()
	sx, sy, sz := live.Position()
	if location.In3DRadius(sx, sy, sz, x, y, z, castRange) {
		return false
	}
	if req.ShiftPressed {
		refuseCastTooFar(live)
		return true
	}
	if live.move == nil {
		sendMagicActionFailed(live)
		return true
	}
	live.clearParkedApproaches()
	live.deferMagicSkill(req, selected)
	accepted, err := live.move.MoveToLocation(location.Location{X: x, Y: y, Z: z})
	if err != nil {
		l.log.Warn().Err(err).Msg("move: broadcast")
	}
	if accepted {
		return true
	}
	live.takeDeferredMagicSkill()
	sendMagicActionFailed(live)
	return true
}

// walkToCastTarget runs the approach a player's CAST intention takes toward
// its creature or door target before anything is paid: nothing while target sits
// within castRange plus both footprints (3D), or when the skill has no range
// or targets the caster. Out of range, a shift-held cast is refused with
// TARGET_TOO_FAR and the player goes idle, walking nowhere; otherwise the
// player walks to target with MoveToPawn at castRange, park storing the cast
// as the intention the walk's arrival thinks again. A player that cannot walk
// is answered ActionFailed and nothing is parked. It reports whether the cast
// must not start now. The caller has already dropped every approach an
// earlier intention parked.
func (l *GameClientLink) walkToCastTarget(live *livePlayer, target skilltarget.Actor, castRange int, shift bool, park func()) bool {
	pawn, radius, ok := castApproachPawn(target)
	if !ok || castRange < 0 || pawn.ObjectID() == live.ObjectID() {
		return false
	}
	lx, ly, lz := live.Position()
	tx, ty, tz := pawn.Position()
	if location.In3DRadius(lx, ly, lz, tx, ty, tz, int(float64(castRange)+live.CollisionRadius()+radius)) {
		return false
	}
	if shift {
		refuseCastTooFar(live)
		return true
	}
	// The specified behavior leaves an immobile caster's CAST intention
	// current with no packet; the request still owes its client an answer.
	if live.move == nil || live.MovementDisabled() {
		sendMagicActionFailed(live)
		return true
	}
	park()
	if !live.move.MoveToPawn(pawn, castRange) {
		live.clearParkedApproaches()
		sendMagicActionFailed(live)
	}
	return true
}

// castApproachPawn returns the final target a cast approach walks to and
// the body radius its range check adds: a creature's collision radius, or
// the fixed radius every door has. Any other target is not approached.
func castApproachPawn(target skilltarget.Actor) (move.Pawn, float64, bool) {
	switch t := target.(type) {
	case attackable.Combatant:
		return t, t.CollisionRadius(), true
	case *door.Object:
		return t, door.CollisionRadius, true
	}
	return nil, 0, false
}

// refuseCastTooFar answers a shift-held cast out of range: TARGET_TOO_FAR,
// and the player goes idle, stopping any walk under way.
func refuseCastTooFar(live *livePlayer) {
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetTooFar))
	live.tryToIdle(false)
	if live.move != nil {
		live.move.Stop()
	}
}

func (l *GameClientLink) resolveMagicSkillTarget(caster actorcast.Target, selected world.Tracked, def modelskill.Definition, ctrl bool) (actorcast.Target, skilltarget.CastRejection) {
	casterCreature, ok := caster.(skilltarget.Actor)
	if !ok {
		return nil, skilltarget.CastRejectNone
	}
	selectedCreature, _ := selected.(skilltarget.Actor)
	handler, ok := l.targets.Handler(def.Target)
	if !ok {
		return nil, skilltarget.CastRejectNone
	}
	finalTarget := handler.FinalTarget(casterCreature, selectedCreature, &def)
	// A classified rejection keeps finalTarget so the cast stops the caster
	// and checks costs and line of sight before reporting it.
	if rejection := skilltarget.CastRejectionFor(def.Target, casterCreature, finalTarget, &def, ctrl); rejection != skilltarget.CastRejectNone {
		return finalTarget, rejection
	}
	// Ground LOS/peace/heading run after cost validation via AfterCanCast.
	return finalTarget, skilltarget.CastRejectNone
}

func sendTargetCastRejection(live *livePlayer, rejection skilltarget.CastRejection, def modelskill.Definition) {
	if live == nil {
		return
	}
	switch rejection {
	case skilltarget.CastRejectInvalidTarget:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
	case skilltarget.CastRejectCantAttackPeaceZone:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCantAtkPeacezone))
	case skilltarget.CastRejectTargetInPeaceZone:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetInPeacezone))
	case skilltarget.CastRejectCannotUseSkill:
		live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageS1CannotBeUsed, int32(def.ID), int32(def.Level)))
	case skilltarget.CastRejectHarvestNotMonster:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageHarvestFailedSeedNotSown))
	case skilltarget.CastRejectCorpseTooOld:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCorpseTooOldSkillNotUsed))
	case skilltarget.CastRejectSweepNotMonster, skilltarget.CastRejectSweepNotSpoiled:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSweeperFailedTargetNotSpoiled))
	case skilltarget.CastRejectSweepNotAllowed:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSweepNotAllowed))
	case skilltarget.CastRejectCannotUseOnYourself:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotUseOnYourself))
	case skilltarget.CastRejectOlympiadUnavailable:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSkillUnavailableForOlympiad))
	}
}

// finishDeferredMagicSkill runs the skill request queued as the next CAST
// intention, if any, and reports whether one was waiting. During a sit-down
// or stand-up the request stays queued for PostureSettled.
func (l *GameClientLink) finishDeferredMagicSkill(live *livePlayer) bool {
	if live == nil || live.detached() {
		return false
	}
	if inPostureTransition(live) {
		return live.hasDeferredMagicSkill()
	}
	return l.runDeferredMagicSkill(live)
}

// runDeferredMagicSkill runs the queued skill request, if any, whatever the
// posture, and reports whether one was waiting.
func (l *GameClientLink) runDeferredMagicSkill(live *livePlayer) bool {
	queued := live.takeDeferredMagicSkill()
	if queued == nil {
		return false
	}
	l.resumeMagicSkill(live, *queued)
	return true
}

// resumeMagicSkill re-evaluates a queued skill request once the swing, cast
// or walk it waited on is over. Its client already had its ActionFailed when
// it was queued, so it answers less than a fresh request does: ActionFailed
// alone when the caster cannot act or is casting again, nothing when the
// target is gone, and a pre-attempt gate failure's reason with no
// ActionFailed. Past those it starts like a fresh request.
func (l *GameClientLink) resumeMagicSkill(live *livePlayer, queued deferredMagicSkill) {
	req := queued.req
	def, known := l.skills.Definition(modelskill.Ref{ID: modelskill.ID(req.SkillID), Level: live.SkillLevel(int(req.SkillID))})
	if !known || (def.Activation != modelskill.ActivationActive && def.Activation != modelskill.ActivationToggle) {
		return
	}
	if live.Character.Dead() || live.Teleporting() || live.Operating() || live.Character.ObserverMode() ||
		live.Character.AllSkillsDisabled() || live.CastingNow() {
		sendMagicActionFailed(live)
		return
	}
	target := l.magicSkillFinalTarget(live, def, queued.selected)
	if target == nil || l.magicTargetLost(live, target, def) {
		return
	}
	if err := l.castController(live).CanPlayerAttemptCast(live.Character, target, def); err != nil {
		sendMagicCastFailureReason(live, def, err)
		return
	}
	// A servitor request keeps waiting on a pets-row read that outlived the
	// swing or cast it was queued behind; the read's end runs it.
	if l.restoringServitor(live, def) {
		live.deferMagicSkill(req, queued.selected)
		return
	}
	l.castMagicSkill(live, req, def, true, queued.selected)
}

// restoringServitor reports whether def summons a servitor while live's
// pets-row read is still in flight.
func (l *GameClientLink) restoringServitor(live *livePlayer, def modelskill.Definition) bool {
	return actorcast.ServitorSummon(def) && l.restoringSummon(live)
}

// resumeServitorAfterRestore runs a queued servitor request once live's
// pets-row read has ended, unless a swing or cast still holds it for its own
// end. Only a servitor request waits on the read, so any other queued
// request is left for whatever it waits on.
func (l *GameClientLink) resumeServitorAfterRestore(live *livePlayer) {
	if live == nil || itemAICastBusy(live) {
		return
	}
	skillID, ok := live.deferredMagicSkillID()
	if !ok {
		return
	}
	def, known := l.skills.Definition(modelskill.Ref{ID: modelskill.ID(skillID), Level: live.SkillLevel(int(skillID))})
	if known && actorcast.ServitorSummon(def) {
		l.finishDeferredMagicSkill(live)
	}
}

// magicTargetLost reports whether a queued skill's target has left the world
// or the caster's surroundings (an invisible player only a game master
// knows). A summon-friend skill reaches a target
// anywhere in the world.
func (l *GameClientLink) magicTargetLost(live *livePlayer, target skilltarget.Actor, def modelskill.Definition) bool {
	tracked, ok := target.(world.Tracked)
	if !ok || l.resolveTarget(target.ObjectID()) == nil {
		return true
	}
	if def.SkillType == "SUMMON_FRIEND" {
		return false
	}
	if c, ok := target.(attackable.Combatant); ok {
		return !live.Knows(c)
	}
	return !world.Knows(live, tracked)
}

func (l *GameClientLink) abortFusionTargeting(target *livePlayer) {
	if l == nil || l.world == nil || target == nil {
		return
	}
	for _, obj := range l.world.Objects() {
		caster, ok := obj.(*livePlayer)
		if ok && caster.fusesTarget(target.ObjectID()) {
			caster.Character.StopCast()
		}
	}
}

// handleMagicSkillUseGround records the client-supplied ground-click point
// on the caster, height-snapped to geodata, then runs the
// same cast pipeline an ordinary RequestMagicSkillUse drives — the ground
// point itself is carried out-of-band via live.Character, not as this
// cast's resolved target.
func (l *GameClientLink) handleMagicSkillUseGround(live *livePlayer, req clientpackets.RequestExMagicSkillUseGround) {
	if live == nil {
		sendMagicActionFailed(live)
		return
	}
	level := live.SkillLevel(int(req.SkillID))
	def, ok := l.skills.Definition(modelskill.Ref{ID: modelskill.ID(req.SkillID), Level: level})
	// RequestExMagicSkillUseGround silently ignores unknown/non-GROUND skills before a pending action or point is recorded.
	if level == 0 || !ok || def.Target != modelskill.TargetGround {
		return
	}
	z := int(req.Z)
	if l.geo != nil {
		z = int(l.geo.Height(int(req.X), int(req.Y), int(req.Z)))
	}
	live.Character.SetGroundTarget(int(req.X), int(req.Y), z)
	l.handleMagicSkillUse(live, clientpackets.RequestMagicSkillUse{
		SkillID:      req.SkillID,
		CtrlPressed:  req.CtrlPressed,
		ShiftPressed: req.ShiftPressed,
	})
}

// handleToggleSkillUse applies casting a toggle skill: an already-active
// instance turns off at no cost, an inactive one pays its MP/HP cost and
// turns on. A toggle's cast window is instantaneous — there is no cast bar,
// no launch packet, and activating one never installs a reuse delay — so
// this bypasses the timed Start/Hit/Finish sequence handleMagicSkillUse
// drives for an ordinary active skill. The on/off decision happens inside
// actorcast.ApplyToggle, but effect application/removal is this handler's
// job, done only after the MagicSkillUse ack goes out — on both branches,
// the broadcast precedes both the skill call and the effect exit. The ack
// is handed to ApplyToggle rather than sent on return, because it is also
// broadcast ahead of the MP/HP consume, and a cost that kills the caster
// sends its own packets from inside that consume.
func (l *GameClientLink) handleToggleSkillUse(live *livePlayer, req clientpackets.RequestMagicSkillUse, selected world.Tracked) {
	handlers := l.castEffects()
	def, target, activated, err := actorcast.ApplyToggle(
		handlers,
		l.castController(live),
		actorcast.PlayerToggleRequest{
			Caster:      live.Character,
			Selected:    selected,
			SkillID:     int(req.SkillID),
			Definitions: l.skills,
		},
		l.stopMovementForCast(live),
		func(accepted modelskill.Definition) {
			selfObject := skillCastObject(live)
			l.broadcastLiveFrame(live, func() wire.Frame {
				return serverpackets.FrameMagicSkillUse(selfObject, selfObject, int32(accepted.ID), int32(accepted.Level), 0, 0, false)
			})
		},
	)
	if err != nil {
		if errors.Is(err, actorcast.ErrNotEnoughMP) || errors.Is(err, actorcast.ErrNotEnoughHP) {
			sendMagicCastFailureReason(live, def, err)
			l.broadcastCastAborted(live)
			sendMagicActionFailed(live)
			live.endCastIntention(def, castCombatant(target), false)
			return
		}
		sendMagicCastFailure(live, def, err)
		return
	}
	// A toggle's cast ends as soon as it has switched, with no CastFinished.
	defer live.endCastIntention(def, castCombatant(target), false)

	if activated {
		// Each cost CastToggle paid already sent its own status.
		handlers.Sink = l.playerMessageSink(live, nil)
		result := actorcast.ApplyEffectsResult(handlers, live.Character, target, def)
		l.syncCubicTargets(live, result, def)
	} else {
		skillhandler.StopEffect(live.Character, def.ID)
	}
}

// broadcastCastAborted tells the caster and everyone watching it that an
// in-flight cast was cancelled: the cancel animation goes to the whole
// known list. The action-failed acknowledgement is not sent here: it belongs
// to every Stop call, idle or in-flight, so it is wired through the
// CastStopAck event instead of gated behind this in-flight-only path. An
// interrupt's CASTING_INTERRUPTED comes last, after that acknowledgement,
// once the stopped cast's CastFinished has run (see livePlayer.endCastStop).
func (l *GameClientLink) broadcastCastAborted(live *livePlayer) {
	if live == nil {
		return
	}
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameMagicSkillCanceled(live.ObjectID())
	})
}

func skillCastObject(obj actorcast.Target) serverpackets.SkillCastObject {
	x, y, z := obj.Position()
	return serverpackets.SkillCastObject{
		ObjectID: obj.ObjectID(),
		Location: location.Location{X: x, Y: y, Z: z},
	}
}

// sendMagicCastFailure rejects a cast that never started: the reason, then
// the action-failed acknowledgement releasing the client's pending action.
func sendMagicCastFailure(live *livePlayer, def modelskill.Definition, err error) {
	sendMagicCastFailureReason(live, def, err)
	sendMagicActionFailed(live)
}

// sendItemConsumeFailure rejects an item-triggered cast whose required item
// could not be destroyed (a stack-destroy race, not the skill's own
// itemConsumeId precheck): NOT_ENOUGH_ITEMS (351), as for any failed item
// destroy on a playable's cast, then the action-failed acknowledgement.
func sendItemConsumeFailure(live *livePlayer) {
	if live == nil {
		return
	}
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughItems))
	sendMagicActionFailed(live)
}

// sendSkillItemCharge tells the caster what its cast's own consume item cost,
// once the cast has been announced: the item that disappeared, or, for an
// item-carried cast that could no longer pay it, that there were not enough.
// Adena reads as adena spent, and a shadow item as its mana running out.
func sendSkillItemCharge(live *livePlayer, def modelskill.Definition, charge actorcast.ItemCharge) {
	if live == nil || charge == actorcast.ItemChargeNone {
		return
	}
	itemID := int32(def.ItemConsumeID)
	count := int32(def.ItemConsumeCount)
	if charge == actorcast.ItemChargeShort {
		if itemID == item.AdenaID {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouNotEnoughAdena))
			return
		}
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughItems))
		return
	}
	switch {
	case itemID == item.AdenaID:
		live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageS1DisappearedAdena, count))
	case shadowTemplate(live, itemID):
		live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageRemainingManaIsNow0, itemID))
	case count > 1:
		live.SendFrame(serverpackets.FrameSystemMessageItemNameItemNumber(serverpackets.SystemMessageS2S1Disappeared, itemID, count))
	default:
		live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageS1Disappeared, itemID))
	}
}

// shadowTemplate reports whether templateID is a time-limited shadow item.
func shadowTemplate(live *livePlayer, templateID int32) bool {
	inv := live.Inventory()
	if inv == nil || inv.Templates() == nil {
		return false
	}
	tmpl, ok := inv.Templates().Get(templateID)
	return ok && tmpl.Duration > -1
}

// sendMagicCastFailureReason sends the reason alone, for a cast that failed
// mid-flight: the abort funnel that cancels it owns the action-failed
// acknowledgement, so sending one here would duplicate it.
func sendMagicCastFailureReason(live *livePlayer, def modelskill.Definition, err error) {
	if live == nil {
		return
	}
	var condErr *actorcast.ConditionError
	switch {
	case errors.As(err, &condErr):
		sendSkillConditionFailure(live, condErr.Clause, def.ID)
	case errors.Is(err, actorcast.ErrNotEnoughMP):
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughMP))
	case errors.Is(err, actorcast.ErrNotEnoughHP):
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughHP))
	case errors.Is(err, actorcast.ErrNotEnoughItems):
		live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageS1CannotBeUsed, int32(def.ID), int32(def.Level)))
	case errors.Is(err, actorcast.ErrCantSeeTarget):
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCantSeeTarget))
	case errors.Is(err, actorcast.ErrWeaponNotAllowed):
		live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageS1CannotBeUsed, int32(def.ID), int32(def.Level)))
	case errors.Is(err, actorcast.ErrOlympiadSkill):
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSkillUnavailableForOlympiad))
	case errors.Is(err, actorcast.ErrSkillDisabled):
		live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageS1PreparedForReuse, int32(def.ID), int32(def.Level)))
	case errors.Is(err, actorcast.ErrAllSkillsDisabled):
		// No reason message: the AI's deny-action check runs before the
		// cast-attempt and skill-disabled checks ever see the actor, so the
		// S1_PREPARED_FOR_REUSE branch is unreachable for a CC'd caster.
		// The player AI's failure reply sends only ActionFailed, which
		// sendMagicCastFailure (above) still sends via
		// sendMagicActionFailed after this reason switch returns.
	case errors.Is(err, actorcast.ErrInvalidTarget):
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
	case errors.Is(err, actorcast.ErrSummonOnlyOne):
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSummonOnlyOne))
	case errors.Is(err, actorcast.ErrSummonInCombat):
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouCannotSummonInCombat))
	case errors.Is(err, actorcast.ErrSummonOnBoat):
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotCallPetFromThisLocation))
	case errors.Is(err, actorcast.ErrFormalWear):
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotUseSkillsWithFormalWear))
	case errors.Is(err, actorcast.ErrFishingSkillsOnly):
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageOnlyFishingSkillsNow))
	case errors.Is(err, actorcast.ErrObserverMode):
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageObserversCannotParticipate))
	case errors.Is(err, actorcast.ErrSitting):
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotMoveWhileSitting))
	case errors.Is(err, actorcast.ErrSiegeSummonUnavailable):
		live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageS1CannotBeUsed, int32(def.ID), int32(def.Level)))
	}
}

// sendSkillConditionFailure sends the feedback a failed skill <cond> clause
// configures: its system message, naming the skill at level 1 when the
// clause asks for the name, or else its literal text, or nothing.
func sendSkillConditionFailure(live *livePlayer, clause modelskill.ConditionClause, skillID modelskill.ID) {
	sendSkillConditionFailureVia(sendFrameTo, live, clause, skillID)
}

// sendSkillConditionFailureVia is sendSkillConditionFailure sending its
// frame through send.
func sendSkillConditionFailureVia(send frameSender, live *livePlayer, clause modelskill.ConditionClause, skillID modelskill.ID) {
	if live == nil {
		return
	}
	switch {
	case clause.MessageID != 0 && clause.AddName:
		send(live, serverpackets.FrameSystemMessageSkillName(int(clause.MessageID), int32(skillID), 1))
	case clause.MessageID != 0:
		send(live, serverpackets.FrameSystemMessage(int(clause.MessageID)))
	case clause.Message != "":
		send(live, serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1, clause.Message))
	}
}

// sendLaunchAbort sends the distinct system message for a launch-phase
// mid-cast revalidation failure. A lost target sends nothing.
func sendLaunchAbort(live *livePlayer, reason actorcast.LaunchAbortReason) {
	if live == nil {
		return
	}
	switch reason {
	case actorcast.LaunchAbortTooFar:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageDistTooFarCastingStopped))
	case actorcast.LaunchAbortNoLineOfSight:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCantSeeTarget))
	case actorcast.LaunchAbortCasterPeaceZone:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCantAtkPeacezone))
	case actorcast.LaunchAbortTargetPeaceZone:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetInPeacezone))
	}
}

func sendMagicActionFailed(live *livePlayer) {
	if live != nil {
		live.SendFrame(serverpackets.FrameActionFailed())
	}
}

// DeliverHitResult forwards a caster's target-addressed skill-handler
// messages (MagicResist, ManaDrain, ...) to their online target when the
// caster itself has no live connection to send caster-addressed messages
// through — a hostile NPC's AIController.OnHitResult (issue #2350).
func (l *GameClientLink) DeliverHitResult(result actorcast.EffectResult) {
	l.sendSkillHandlerResult(nil, result)
}

// HostileCastEffects returns the effect handlers a hostile NPC's AI cast
// dispatches through: the link's own target and skill registries (both
// read-only after construction, so NPC and player queues share them) and
// DeliverHitResult for target-addressed messages.
func (l *GameClientLink) HostileCastEffects() actorcast.EffectHandlers {
	handlers := l.castEffects()
	handlers.OnHitResult = l.DeliverHitResult
	return handlers
}

// playerMessageSink delivers a player caster's skill-handler messages as the
// handler produces them, so each keeps its place among the frames the hit
// itself sends (the target's status, death, the kill's rewards). onStatus,
// when set, runs after each message that sent live its own status.
//
// The sink runs on live's queue, so it first runs the PvP flag changes
// pending for live: a PK kill the hit just made takes its items off and
// resets its flag before the message that follows the killing blow.
func (l *GameClientLink) playerMessageSink(live *livePlayer, onStatus func()) skillhandler.MessageSink {
	return func(message any) {
		l.settlePvPChanges(live)
		if l.sendSkillHandlerResult(live, actorcast.EffectResult{Messages: []any{message}}) && onStatus != nil {
			onStatus()
		}
	}
}

// castEffects returns the effect handlers a cast dispatches through: the
// link's target and skill registries and its chance procs, all read-only
// after construction and shared by every actor's queue.
func (l *GameClientLink) castEffects() actorcast.EffectHandlers {
	return actorcast.EffectHandlers{Targets: l.targets, Skills: l.skillHandlers, Chance: l.chance}
}

// frameSender sends frame to recipient's client.
type frameSender func(recipient *livePlayer, frame wire.Frame)

// sendFrameTo sends frame to recipient at once.
func sendFrameTo(recipient *livePlayer, frame wire.Frame) {
	recipient.SendFrame(frame)
}

// sendSkillHandlerResult delivers both caster-addressed messages (sent to
// live, when connected) and target-addressed messages (resolved by ID
// through l.livePlayerByID, independent of whether live is connected or
// even nil) from a resolved skill-handler result. It reports whether it sent
// live its own status, so a caller that follows with a changed-vitals
// StatusUpdate can measure from there instead of repeating it.
func (l *GameClientLink) sendSkillHandlerResult(live *livePlayer, result actorcast.EffectResult) (statusSent bool) {
	return l.sendSkillHandlerResultVia(sendFrameTo, live, result)
}

// sendSkillHandlerResultVia is sendSkillHandlerResult sending each frame
// through send.
func (l *GameClientLink) sendSkillHandlerResultVia(send frameSender, live *livePlayer, result actorcast.EffectResult) (statusSent bool) {
	for _, message := range result.Messages {
		switch m := message.(type) {
		case skillhandler.CasterVitalsChanged:
			if live != nil {
				send(live, liveStatusFrame(live))
				statusSent = true
			}
		case skillhandler.Counterattack:
			attacker, attackerOnline := l.livePlayerByID(m.AttackerID)
			defender, defenderOnline := l.livePlayerByID(m.DefenderID)
			attackerName := m.AttackerName
			if attackerOnline {
				attackerName = attacker.Name
			}
			defenderName := m.DefenderName
			if defenderOnline {
				defenderName = defender.Name
			}
			if defenderOnline {
				send(defender, serverpackets.FrameSystemMessageString(serverpackets.SystemMessageCounteredS1Attack, attackerName))
			}
			if attackerOnline {
				send(attacker, serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1PerformingCounterattack, defenderName))
			}
		case skillhandler.Dodge:
			attacker, attackerOnline := l.livePlayerByID(m.AttackerID)
			defender, defenderOnline := l.livePlayerByID(m.DefenderID)
			attackerName := m.AttackerName
			if attackerOnline {
				attackerName = attacker.Name
			}
			defenderName := m.DefenderName
			if defenderOnline {
				defenderName = defender.Name
			}
			if attackerOnline {
				send(attacker, serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1DodgesAttack, defenderName))
			}
			if defenderOnline {
				send(defender, serverpackets.FrameSystemMessageString(serverpackets.SystemMessageAvoidedS1Attack, attackerName))
			}
		case skillhandler.Lethal:
			if target, online := l.livePlayerByID(m.TargetID); online {
				send(target, serverpackets.FrameSystemMessage(serverpackets.SystemMessageLethalStrike))
			}
			if attacker, online := l.livePlayerByID(m.AttackerID); online {
				send(attacker, serverpackets.FrameSystemMessage(serverpackets.SystemMessageLethalStrikeSuccessful))
			}
		case skillhandler.Damage:
			if recipient, online := l.livePlayerByID(m.RecipientID); online {
				sendDamageMessageVia(send, recipient, m)
			}
		case skillhandler.DamageReceived:
			if target, online := l.livePlayerByID(m.TargetID); online {
				send(target, serverpackets.FrameSystemMessageStringNumber(serverpackets.SystemMessageS1GaveYouS2Dmg, m.AttackerName, m.Amount))
			}
		case skillhandler.Resisted:
			if live != nil {
				send(live, serverpackets.FrameSystemMessageStringSkillName(serverpackets.SystemMessageS1ResistedYourS2, m.TargetName, int32(m.SkillID), int32(m.SkillLevel)))
			}
		case skillhandler.AttackFailedMessage:
			if live != nil {
				send(live, serverpackets.FrameSystemMessage(serverpackets.SystemMessageAttackFailed))
			}
		case skillhandler.DrainHalfSucceededMessage:
			if live != nil {
				send(live, serverpackets.FrameSystemMessage(serverpackets.SystemMessageDrainHalfSuccessful))
			}
		case skillhandler.DoorUnlockUnableMessage:
			if live != nil {
				send(live, serverpackets.FrameSystemMessage(serverpackets.SystemMessageUnableToUnlockDoor))
			}
		case skillhandler.DoorUnlockFailedMessage:
			if live != nil {
				send(live, serverpackets.FrameSystemMessage(serverpackets.SystemMessageFailedToUnlockDoor))
			}
		case skillhandler.InvalidTargetMessage:
			if live != nil {
				send(live, serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
			}
		case skillhandler.RecipeBookOpened:
			if live != nil {
				send(live, recipeBookFrame(live, m.Dwarven))
			}
		case skillhandler.CraftWhileOperatingMessage:
			if live != nil {
				send(live, serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotCreateWhileTrading))
			}
		case skillhandler.FishingCast:
			if live != nil {
				l.castFishing(live)
			}
		case skillhandler.FishingAction:
			if live != nil {
				l.useFishingAction(live, m)
			}
		case skillhandler.SlotsFullMessage:
			if live != nil {
				send(live, serverpackets.FrameSystemMessage(serverpackets.SystemMessageSlotsFull))
			}
		case skillhandler.NothingInsideMessage:
			if live != nil {
				send(live, serverpackets.FrameSystemMessage(serverpackets.SystemMessageNothingInsideThat))
			}
		case skillhandler.ManorMessage:
			if live != nil {
				send(live, manorMessageFrame(m))
			}
		case skillhandler.CropHarvested:
			if live != nil {
				l.sendCropHarvested(live, m)
			}
		case skillhandler.MagicResist:
			target, online := l.livePlayerByID(m.TargetID)
			if !online {
				continue
			}
			id := serverpackets.SystemMessageResistedS1Magic
			if m.Drain {
				id = serverpackets.SystemMessageResistedS1Drain
			}
			send(target, serverpackets.FrameSystemMessageString(id, m.AttackerName))
		case skillhandler.ManaDamageMissedMessage:
			if live != nil {
				send(live, serverpackets.FrameSystemMessage(serverpackets.SystemMessageMissedTarget))
			}
		case skillhandler.ManaDrain:
			target, online := l.livePlayerByID(m.TargetID)
			if !online {
				continue
			}
			send(target, serverpackets.FrameSystemMessageStringNumber(serverpackets.SystemMessageS2MPHasBeenDrainedByS1, m.CasterName, m.MP))
		case skillhandler.OpponentMPReducedMessage:
			if live != nil {
				send(live, serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageYourOpponentsMPWasReducedByS1, m.MP))
			}
		}
	}
	return statusSent
}

// sendDamageMessage sends a skill or auto-attack hit's damage feedback: a
// player sees each critical kind it rolled, a summon's owner sees one summon
// critical, then either the blocked notice or the damage dealt.
func sendDamageMessage(recipient *livePlayer, m skillhandler.Damage) {
	sendDamageMessageVia(sendFrameTo, recipient, m)
}

// sendDamageMessageVia is sendDamageMessage sending each frame through
// send.
func sendDamageMessageVia(send frameSender, recipient *livePlayer, m skillhandler.Damage) {
	crit := m.PhysicalCrit || m.MagicCrit
	dealt := serverpackets.SystemMessageYouDidS1Dmg
	switch m.Source {
	case skillhandler.DamageByPlayer:
		if m.PhysicalCrit {
			send(recipient, serverpackets.FrameSystemMessage(serverpackets.SystemMessageCriticalHit))
		}
		if m.MagicCrit {
			send(recipient, serverpackets.FrameSystemMessage(serverpackets.SystemMessageCriticalHitMagic))
		}
	case skillhandler.DamageByPet:
		dealt = serverpackets.SystemMessagePetHitForS1Damage
		if crit {
			send(recipient, serverpackets.FrameSystemMessage(serverpackets.SystemMessageCriticalHitByPet))
		}
	case skillhandler.DamageByServitor:
		dealt = serverpackets.SystemMessageSummonGaveDamageS1
		if crit {
			send(recipient, serverpackets.FrameSystemMessage(serverpackets.SystemMessageCriticalHitBySummonedMob))
		}
	}
	switch {
	case m.Petrified:
		send(recipient, serverpackets.FrameSystemMessage(serverpackets.SystemMessageOpponentPetrified))
	case m.Blocked:
		send(recipient, serverpackets.FrameSystemMessage(serverpackets.SystemMessageAttackWasBlocked))
	default:
		send(recipient, serverpackets.FrameSystemMessageNumber(dealt, m.Amount))
	}
}

func sendMagicStatusUpdate(live *livePlayer, before player.Vitals) {
	if live == nil {
		return
	}
	attrs := magicStatusAttributes(before.ChangesTo(live.Vitals()))
	if len(attrs) > 0 {
		live.SendFrame(serverpackets.FrameStatusUpdate(live.ObjectID(), attrs))
	}
}

func magicStatusAttributes(change player.VitalsChange) []serverpackets.StatusAttribute {
	if !change.Changed() {
		return nil
	}
	attrs := make([]serverpackets.StatusAttribute, 0, 2)
	if change.HPChanged {
		attrs = append(attrs, serverpackets.StatusAttribute{Type: serverpackets.StatusCurrentHP, Value: change.HP})
	}
	if change.MPChanged {
		attrs = append(attrs, serverpackets.StatusAttribute{Type: serverpackets.StatusCurrentMP, Value: change.MP})
	}
	return attrs
}

func millis(d time.Duration) int {
	return int(d / time.Millisecond)
}
