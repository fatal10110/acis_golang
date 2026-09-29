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
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
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
		if castable && !l.attemptMagicSkill(live, def, selected) {
			return
		}
	}
	// A pets-row read still in flight stands in for a cast the caster is
	// still in, whether or not its hold already ended. A servitor cast started
	// there would pay its item and MP, only to lose the slot to the inbound
	// pet at hit, so it is refused, not queued, with an ActionFailed and no
	// reason: queued, it would run once the pet has landed and pay for a
	// cast refused at hit, until the servitor gate refuses it before any
	// cost (#2513). The pre-attempt gate above still answers first, with its
	// own reason, as it would for any caster.
	restoringServitor := known && def.SkillType == "SUMMON" && !def.IsCubic && l.restoringSummon(live)
	// A request that passed the pre-attempt gate while a swing or another
	// cast is in flight, toggle or not, becomes the next CAST intention and
	// is answered with ActionFailed; the swing's or cast's end runs it.
	// Starting it now would pay its costs and broadcast MagicSkillUse (or
	// switch a toggle) before what is in flight has finished. It replaces an
	// attack queued behind the same cast, and the attack the swing in
	// flight is for. itemAICastBusy is the wait predicate every cast request
	// shares, the sit-down and stand-up transitions included.
	if castable && !restoringServitor && itemAICastBusy(live) {
		live.deferMagicSkill(req, selected)
		if live.combat != nil {
			live.combat.ReplaceWithCast()
		}
		sendMagicActionFailed(live)
		return
	}
	if restoringServitor {
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
	if known && def.Activation == modelskill.ActivationToggle {
		l.handleToggleSkillUse(live, req, selected)
		return
	}
	live.Character.SetCastModifiers(req.CtrlPressed, req.ShiftPressed)
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
		// A player's cast that fails its cost or target conditions after the
		// hit-time stop answers with its reason alone: no ActionFailed, no
		// heading toward the target and no MoveToPawn (those belong to the
		// pet and NPC cast paths). A locked door is refused with no packet
		// at all, matching the reference's silent return from its target
		// check.
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
	if def.SkillType == "FUSION" {
		l.startFusionCast(live, controller, handlers, target, def, plan)
		return
	}

	l.broadcastCastStart(live, target, def, plan)
	if plan.GaugeDuration > 0 {
		live.SendFrame(serverpackets.FrameSetupGauge(serverpackets.GaugeBlue, millis(plan.GaugeDuration), millis(plan.GaugeDuration)))
	}

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
func (l *GameClientLink) startFusionCast(live *livePlayer, controller *actorcast.Controller, handlers actorcast.EffectHandlers, target actorcast.Target, def modelskill.Definition, plan actorcast.Plan) {
	live.setFusionTarget(target.ObjectID())
	finishFusion := func() {
		// Only a creature carries the triggered fusion effect to decrease.
		if effected, ok := target.(attackable.Combatant); ok {
			skillhandler.DecreaseFusion(l.skills, live.Character, effected, def)
		}
		live.clearFusionTarget(target.ObjectID())
	}
	handlers.Sink = l.playerMessageSink(live, nil)
	result := actorcast.ApplyEffectsResult(handlers, live.Character, target, def)
	l.syncCubicTargets(live, result, def)

	l.broadcastCastStart(live, target, def, plan)
	live.SendFrame(serverpackets.FrameSetupGauge(serverpackets.GaugeBlue, millis(plan.HitTime), millis(plan.HitTime)))
	if !controller.ScheduleFusion(plan, time.Second, func() bool {
		return actorcast.FusionChannelValid(live.Character, target, def.CastRange)
	}, finishFusion) {
		finishFusion()
	}
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
// launch resolved. The final MP/HP costs already sent their own statuses,
// and every change the effects make to the caster's vitals reports its own
// status where it happens, so the hit sends none of its own.
func (l *GameClientLink) applyCastHit(live *livePlayer, handlers actorcast.EffectHandlers, affected []skilltarget.Actor, def modelskill.Definition) {
	handlers.Sink = l.playerMessageSink(live, nil)
	result := actorcast.ApplyResolvedEffectsResult(handlers, live.Character, affected, def)
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
		errors.Is(err, actorcast.ErrCubicListFull) ||
		errors.Is(err, actorcast.ErrNotEnoughItems) ||
		errors.Is(err, actorcast.ErrWeaponNotAllowed) ||
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
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetTooFar))
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
	// and checks costs before reporting it. Target conditions are classified
	// here, before any cast-start line-of-sight check; line of sight is only
	// revalidated at launch.
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
	case skilltarget.CastRejectSweepNotMonster:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSweeperFailedTargetNotSpoiled))
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
	if def.SkillType == "SUMMON" && !def.IsCubic && l.restoringSummon(live) {
		sendMagicActionFailed(live)
		return
	}
	l.castMagicSkill(live, req, def, true, queued.selected)
}

// magicTargetLost reports whether a queued skill's target has left the world
// or the caster's surroundings. A summon-friend skill reaches a target
// anywhere in the world.
func (l *GameClientLink) magicTargetLost(live *livePlayer, target skilltarget.Actor, def modelskill.Definition) bool {
	tracked, ok := target.(world.Tracked)
	if !ok || l.resolveTarget(target.ObjectID()) == nil {
		return true
	}
	return def.SkillType != "SUMMON_FRIEND" && !world.Knows(live, tracked)
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
// on the caster, height-snapped to geodata like the reference, then runs the
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
// matching PlayerCast.doToggleCast broadcasting before either callSkill or
// effect.exit() (PlayerCast.java:127 vs 135-137). The ack is handed to
// ApplyToggle rather than sent on return, because the reference also
// broadcasts it ahead of the MP/HP consume (:127 vs :139-165) and a cost
// that kills the caster sends its own packets from inside that consume.
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
			l.broadcastCastAborted(live, false)
			sendMagicActionFailed(live)
			live.endCastIntention(def)
			return
		}
		sendMagicCastFailure(live, def, err)
		return
	}
	// A toggle's cast ends as soon as it has switched, with no CastFinished.
	defer live.endCastIntention(def)

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
// known list. interrupted additionally sends CASTING_INTERRUPTED to the
// caster alone, matching CreatureCast.interrupt() vs the unconditional
// stop(). The action-failed acknowledgement is not sent here: it belongs to
// every Stop call, idle or in-flight (PlayerCast.stop()'s unconditional
// clientActionFailed(), PlayerCast.java:381-387), so it is wired through
// the CastStopAck event instead of gated behind this in-flight-only path.
func (l *GameClientLink) broadcastCastAborted(live *livePlayer, interrupted bool) {
	if live == nil {
		return
	}
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameMagicSkillCanceled(live.ObjectID())
	})
	if interrupted {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCastingInterrupted))
	}
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
// itemConsumeId precheck): NOT_ENOUGH_ITEMS (351), matching Java's
// PlayableCast destroyItem failure, then the action-failed acknowledgement.
func sendItemConsumeFailure(live *livePlayer) {
	if live == nil {
		return
	}
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughItems))
	sendMagicActionFailed(live)
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
	case errors.Is(err, actorcast.ErrSkillDisabled):
		live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageS1PreparedForReuse, int32(def.ID), int32(def.Level)))
	case errors.Is(err, actorcast.ErrAllSkillsDisabled):
		// No reason message: PlayableAI.tryToCast's denyAiAction() check (Java
		// PlayableAI.java:299-303) runs before canAttemptCast/isSkillDisabled
		// ever sees the actor, so the S1_PREPARED_FOR_REUSE branch
		// (CreatureCast.java:324-327) is unreachable for a CC'd caster.
		// PlayerAI.clientActionFailed() (PlayerAI.java:556-560) sends only
		// ActionFailed, which sendMagicCastFailure (above) still sends via
		// sendMagicActionFailed after this reason switch returns.
	case errors.Is(err, actorcast.ErrInvalidTarget):
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
	case errors.Is(err, actorcast.ErrCubicListFull):
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCubicSummoningFailed))
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
	if live == nil {
		return
	}
	switch {
	case clause.MessageID != 0 && clause.AddName:
		live.SendFrame(serverpackets.FrameSystemMessageSkillName(int(clause.MessageID), int32(skillID), 1))
	case clause.MessageID != 0:
		live.SendFrame(serverpackets.FrameSystemMessage(int(clause.MessageID)))
	case clause.Message != "":
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1, clause.Message))
	}
}

// sendLaunchAbort sends the reference's distinct system message for a
// launch-phase mid-cast revalidation failure. A lost target sends nothing,
// matching CreatureCast.onMagicLaunch.
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
func (l *GameClientLink) playerMessageSink(live *livePlayer, onStatus func()) skillhandler.MessageSink {
	return func(message any) {
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

// sendSkillHandlerResult delivers both caster-addressed messages (sent to
// live, when connected) and target-addressed messages (resolved by ID
// through l.livePlayerByID, independent of whether live is connected or
// even nil) from a resolved skill-handler result. It reports whether it sent
// live its own status, so a caller that follows with a changed-vitals
// StatusUpdate can measure from there instead of repeating it.
func (l *GameClientLink) sendSkillHandlerResult(live *livePlayer, result actorcast.EffectResult) (statusSent bool) {
	for _, message := range result.Messages {
		switch m := message.(type) {
		case skillhandler.CasterVitalsChanged:
			if live != nil {
				sendLiveStatus(live)
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
				defender.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageCounteredS1Attack, attackerName))
			}
			if attackerOnline {
				attacker.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1PerformingCounterattack, defenderName))
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
				attacker.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1DodgesAttack, defenderName))
			}
			if defenderOnline {
				defender.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageAvoidedS1Attack, attackerName))
			}
		case skillhandler.Lethal:
			if target, online := l.livePlayerByID(m.TargetID); online {
				target.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageLethalStrike))
			}
			if attacker, online := l.livePlayerByID(m.AttackerID); online {
				attacker.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageLethalStrikeSuccessful))
			}
		case skillhandler.Damage:
			if recipient, online := l.livePlayerByID(m.RecipientID); online {
				sendDamageMessage(recipient, m)
			}
		case skillhandler.DamageReceived:
			if target, online := l.livePlayerByID(m.TargetID); online {
				target.SendFrame(serverpackets.FrameSystemMessageStringNumber(serverpackets.SystemMessageS1GaveYouS2Dmg, m.AttackerName, m.Amount))
			}
		case skillhandler.Resisted:
			if live != nil {
				live.SendFrame(serverpackets.FrameSystemMessageStringSkillName(serverpackets.SystemMessageS1ResistedYourS2, m.TargetName, int32(m.SkillID), int32(m.SkillLevel)))
			}
		case skillhandler.AttackFailedMessage:
			if live != nil {
				live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageAttackFailed))
			}
		case skillhandler.DrainHalfSucceededMessage:
			if live != nil {
				live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageDrainHalfSuccessful))
			}
		case skillhandler.DoorUnlockUnableMessage:
			if live != nil {
				live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageUnableToUnlockDoor))
			}
		case skillhandler.DoorUnlockFailedMessage:
			if live != nil {
				live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageFailedToUnlockDoor))
			}
		case skillhandler.UnlockInvalidTargetMessage:
			if live != nil {
				live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
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
			target.SendFrame(serverpackets.FrameSystemMessageString(id, m.AttackerName))
		case skillhandler.ManaDamageMissedMessage:
			if live != nil {
				live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageMissedTarget))
			}
		case skillhandler.ManaDrain:
			target, online := l.livePlayerByID(m.TargetID)
			if !online {
				continue
			}
			target.SendFrame(serverpackets.FrameSystemMessageStringNumber(serverpackets.SystemMessageS2MPHasBeenDrainedByS1, m.CasterName, m.MP))
		case skillhandler.OpponentMPReducedMessage:
			if live != nil {
				live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageYourOpponentsMPWasReducedByS1, m.MP))
			}
		}
	}
	return statusSent
}

// sendDamageMessage sends a skill or auto-attack hit's damage feedback: a
// player sees each critical kind it rolled, a summon's owner sees one summon
// critical, then either the blocked notice or the damage dealt.
func sendDamageMessage(recipient *livePlayer, m skillhandler.Damage) {
	crit := m.PhysicalCrit || m.MagicCrit
	dealt := serverpackets.SystemMessageYouDidS1Dmg
	switch m.Source {
	case skillhandler.DamageByPlayer:
		if m.PhysicalCrit {
			recipient.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCriticalHit))
		}
		if m.MagicCrit {
			recipient.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCriticalHitMagic))
		}
	case skillhandler.DamageByPet:
		dealt = serverpackets.SystemMessagePetHitForS1Damage
		if crit {
			recipient.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCriticalHitByPet))
		}
	case skillhandler.DamageByServitor:
		dealt = serverpackets.SystemMessageSummonGaveDamageS1
		if crit {
			recipient.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCriticalHitBySummonedMob))
		}
	}
	switch {
	case m.Petrified:
		recipient.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageOpponentPetrified))
	case m.Blocked:
		recipient.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageAttackWasBlocked))
	default:
		recipient.SendFrame(serverpackets.FrameSystemMessageNumber(dealt, m.Amount))
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
