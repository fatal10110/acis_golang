package network

import (
	"errors"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	itemhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/item"
	skillhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
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

	def, known := l.skills.Definition(modelskill.Ref{ID: modelskill.ID(req.SkillID), Level: live.SkillLevel(int(req.SkillID))})
	if known {
		if itemhandler.RecallCastBlockedByKarma(def.SkillType, live.Karma(), l.playerConfig.KarmaPlayerCanTeleport) {
			sendMagicActionFailed(live)
			return
		}
		if def.Activation == modelskill.ActivationToggle {
			l.handleToggleSkillUse(live, req)
			return
		}
	}
	// PlayableAI.tryToCast (aCis:297-317) stores the requested cast as the
	// next intention while a swing is active; starting it here would consume
	// cast resources and broadcast MagicSkillUse before that swing finishes.
	if live.attack != nil && live.attack.AttackingNow() {
		live.deferMagicSkill(req)
		sendMagicActionFailed(live)
		return
	}

	live.Character.SetCastModifiers(req.CtrlPressed, req.ShiftPressed)
	if def.Target == modelskill.TargetGround && l.walkToGroundCast(live, req, def.CastRange) {
		return
	}
	if def.Target == modelskill.TargetGround {
		x, y, z := live.GroundTarget()
		switch skilltarget.GroundCastFailureFor(live.Character, &def) {
		case skilltarget.GroundCastNoLineOfSight:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCantSeeTarget))
			sendMagicActionFailed(live)
			return
		case skilltarget.GroundCastPeaceZone:
			live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageS1CannotBeUsed, int32(def.ID), int32(def.Level)))
			sendMagicActionFailed(live)
			return
		}
		live.Character.SetHeading(live.CurrentLocation().HeadingTo(location.Location{X: x, Y: y, Z: z}))
		l.broadcastLiveFrame(live, func() wire.Frame {
			return serverpackets.FrameValidateLocation(live.ObjectID(), live.CurrentLocation(), live.CurrentHeading())
		})
	}

	beforeVitals := live.Vitals()
	controller := l.castController(live)
	started, err := actorcast.StartPlayerSkill(actorcast.PlayerSkillRequest{
		Now:           time.Now(),
		Controller:    controller,
		Caster:        live.Character,
		Selected:      live.Target(),
		SkillID:       int(req.SkillID),
		Definitions:   l.skills,
		Ctrl:          req.CtrlPressed,
		Shift:         req.ShiftPressed,
		ResolveTarget: l.resolveMagicSkillTarget,
	})
	if err != nil {
		if started.Rejection != skilltarget.CastRejectNone {
			sendTargetCastRejection(live, started.Rejection)
			sendMagicActionFailed(live)
			return
		}
		if errors.Is(err, actorcast.ErrInvalidTarget) && started.Target == nil {
			sendCorpseCastFailure(live, started.Definition)
			sendMagicActionFailed(live)
			return
		}
		sendMagicCastFailure(live, started.Definition, err)
		return
	}
	def = started.Definition
	target := started.Target
	plan := started.Plan

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
	if plan.GaugeDuration > 0 {
		live.SendFrame(serverpackets.FrameSetupGauge(serverpackets.GaugeBlue, millis(plan.GaugeDuration), millis(plan.GaugeDuration)))
	}
	if def.SkillType == "FUSION" {
		live.setFusionTarget(target.ObjectID())
		finishFusion := func() {
			skillhandler.DecreaseFusion(l.skills, live.Character, target, def)
			live.clearFusionTarget(target.ObjectID())
		}
		result := actorcast.ApplyEffectsResult(actorcast.EffectHandlers{Targets: l.targets, Skills: l.skillHandlers}, live.Character, target, def)
		l.sendSkillHandlerResult(live, result)
		l.syncCubicTargets(live, result, def)
		sendMagicStatusUpdate(live, beforeVitals)
		if !controller.ScheduleFusion(plan, time.Second, func() bool {
			return actorcast.FusionChannelValid(live.Character, target, def.CastRange)
		}, finishFusion) {
			finishFusion()
		}
		return
	}

	controller.Schedule(plan, actorcast.Hooks{
		Launch: func() bool {
			if reason := actorcast.RevalidateLaunch(live.Character, target, def); reason != actorcast.LaunchAbortNone {
				sendLaunchAbort(live, reason)
				return false
			}
			handler, ok := l.targets.Handler(def.Target)
			if !ok {
				return false
			}
			resolvedTarget, ok := target.(skilltarget.Creature)
			if !ok {
				return false
			}
			affected := handler.Targets(live.Character, resolvedTarget, &def)
			targetIDs := make([]int32, 0, len(affected))
			for _, affectedTarget := range affected {
				targetIDs = append(targetIDs, affectedTarget.ObjectID())
			}
			l.broadcastLiveFrame(live, func() wire.Frame {
				return serverpackets.FrameMagicSkillLaunched(live.ObjectID(), int32(def.ID), int32(def.Level), targetIDs)
			})
			return true
		},
		Hit: func() {
			result := actorcast.ApplyEffectsResult(actorcast.EffectHandlers{Targets: l.targets, Skills: l.skillHandlers}, live.Character, target, def)
			l.sendSkillHandlerResult(live, result)
			l.syncCubicTargets(live, result, def)
			sendMagicStatusUpdate(live, beforeVitals)
		},
		Failed: func(err error) {
			sendMagicCastFailureReason(live, def, err)
			sendMagicStatusUpdate(live, beforeVitals)
		},
	})
}

func sendCorpseCastFailure(live *livePlayer, def modelskill.Definition) {
	if live == nil || (def.Target != modelskill.TargetCorpseMob && def.Target != modelskill.TargetAreaCorpseMob) {
		return
	}
	target, _ := live.Target().(skilltarget.Creature)
	switch skilltarget.CorpseCastFailureFor(target, &def) {
	case skilltarget.CorpseCastHarvestNotMonster:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageHarvestFailedSeedNotSown))
	case skilltarget.CorpseCastTooOld:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCorpseTooOldSkillNotUsed))
	case skilltarget.CorpseCastSweepNotMonster:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSweeperFailedTargetNotSpoiled))
	}
}

func (l *GameClientLink) walkToGroundCast(live *livePlayer, req clientpackets.RequestMagicSkillUse, castRange int) bool {
	x, y, z := live.GroundTarget()
	sx, sy, sz := live.Position()
	if location.In3DRange(sx, sy, sz, x, y, z, castRange) {
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
	live.takePickup()
	live.takePetInteract()
	live.deferMagicSkill(req)
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
	casterCreature, ok := caster.(skilltarget.Creature)
	if !ok {
		return nil, skilltarget.CastRejectNone
	}
	selectedCreature, _ := selected.(skilltarget.Creature)
	handler, ok := l.targets.Handler(def.Target)
	if !ok {
		return nil, skilltarget.CastRejectNone
	}
	finalTarget := handler.FinalTarget(casterCreature, selectedCreature, &def)
	if rejection := skilltarget.CastRejectionFor(def.Target, casterCreature, finalTarget, &def, ctrl); rejection != skilltarget.CastRejectNone {
		return nil, rejection
	}
	if finalTarget == nil || !handler.CanCast(casterCreature, finalTarget, &def, ctrl) {
		return nil, skilltarget.CastRejectNone
	}
	return finalTarget, skilltarget.CastRejectNone
}

func sendTargetCastRejection(live *livePlayer, rejection skilltarget.CastRejection) {
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
	}
}

func (l *GameClientLink) finishDeferredMagicSkill(live *livePlayer) {
	if live == nil || live.detached() {
		return
	}
	if req := live.takeDeferredMagicSkill(); req != nil {
		l.handleMagicSkillUse(live, *req)
	}
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
// effect.exit() (PlayerCast.java:127 vs 135-137).
func (l *GameClientLink) handleToggleSkillUse(live *livePlayer, req clientpackets.RequestMagicSkillUse) {
	handlers := actorcast.EffectHandlers{Targets: l.targets, Skills: l.skillHandlers}
	def, target, activated, err := actorcast.ApplyToggle(
		handlers,
		l.castController(live),
		actorcast.PlayerToggleRequest{
			Caster:      live.Character,
			Selected:    live.Target(),
			SkillID:     int(req.SkillID),
			Definitions: l.skills,
		},
	)
	broadcast := func() {
		selfObject := skillCastObject(live)
		l.broadcastLiveFrame(live, func() wire.Frame {
			return serverpackets.FrameMagicSkillUse(selfObject, selfObject, int32(def.ID), int32(def.Level), 0, 0, false)
		})
	}
	if err != nil {
		if errors.Is(err, actorcast.ErrNotEnoughMP) || errors.Is(err, actorcast.ErrNotEnoughHP) {
			broadcast()
			sendMagicCastFailureReason(live, def, err)
			l.broadcastCastAborted(live, false)
			sendMagicActionFailed(live)
			return
		}
		sendMagicCastFailure(live, def, err)
		return
	}

	broadcast()
	if activated {
		result := actorcast.ApplyEffectsResult(handlers, live.Character, target, def)
		l.sendSkillHandlerResult(live, result)
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
// Controller.SetOnStopAck instead of gated behind this in-flight-only path.
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
	switch {
	case errors.Is(err, actorcast.ErrNotEnoughMP):
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughMP))
	case errors.Is(err, actorcast.ErrNotEnoughHP):
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughHP))
	case errors.Is(err, actorcast.ErrNotEnoughItems):
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

func (l *GameClientLink) sendSkillHandlerResult(live *livePlayer, result actorcast.EffectResult) {
	if live == nil {
		return
	}
	for _, counterattack := range result.Counterattacks {
		attacker, attackerOnline := l.livePlayerByID(counterattack.AttackerID)
		defender, defenderOnline := l.livePlayerByID(counterattack.DefenderID)
		attackerName := counterattack.AttackerName
		if attackerOnline {
			attackerName = attacker.Name
		}
		defenderName := counterattack.DefenderName
		if defenderOnline {
			defenderName = defender.Name
		}
		if defenderOnline {
			defender.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageCounteredS1Attack, attackerName))
		}
		if attackerOnline {
			attacker.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1PerformingCounterattack, defenderName))
		}
	}
	for _, dodge := range result.Dodges {
		attacker, attackerOnline := l.livePlayerByID(dodge.AttackerID)
		defender, defenderOnline := l.livePlayerByID(dodge.DefenderID)
		attackerName := dodge.AttackerName
		if attackerOnline {
			attackerName = attacker.Name
		}
		defenderName := dodge.DefenderName
		if defenderOnline {
			defenderName = defender.Name
		}
		if attackerOnline {
			attacker.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1DodgesAttack, defenderName))
		}
		if defenderOnline {
			defender.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageAvoidedS1Attack, attackerName))
		}
	}
	for _, lethal := range result.Lethals {
		if target, online := l.livePlayerByID(lethal.TargetID); online {
			target.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageLethalStrike))
		}
		if attacker, online := l.livePlayerByID(lethal.AttackerID); online {
			attacker.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageLethalStrikeSuccessful))
		}
	}
	for _, resisted := range result.Resisted {
		live.SendFrame(serverpackets.FrameSystemMessageStringSkillName(serverpackets.SystemMessageS1ResistedYourS2, resisted.TargetName, int32(resisted.SkillID), int32(resisted.SkillLevel)))
	}
	for i := 0; i < result.AttackFailed; i++ {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageAttackFailed))
	}
	for _, resist := range result.MagicResists {
		target, online := l.livePlayerByID(resist.TargetID)
		if !online {
			continue
		}
		target.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageResistedS1Magic, resist.AttackerName))
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
