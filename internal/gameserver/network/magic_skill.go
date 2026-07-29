package network

import (
	"errors"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	itemhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/item"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func (l *GameClientLink) handleMagicSkillUse(live *livePlayer, req clientpackets.RequestMagicSkillUse) {
	if live == nil {
		sendMagicActionFailed(live)
		return
	}

	if def, ok := l.skills.Definition(modelskill.Ref{ID: modelskill.ID(req.SkillID), Level: live.SkillLevel(int(req.SkillID))}); ok {
		if itemhandler.RecallCastBlockedByKarma(def.SkillType, live.Karma(), l.playerConfig.KarmaPlayerCanTeleport) {
			sendMagicActionFailed(live)
			return
		}
		if def.Activation == modelskill.ActivationToggle {
			l.handleToggleSkillUse(live, req)
			return
		}
	}

	beforeVitals := live.Vitals()
	controller := l.castController(live)
	started, err := actorcast.StartPlayerSkill(actorcast.PlayerSkillRequest{
		Now:         time.Now(),
		Controller:  controller,
		Caster:      live.Character,
		Selected:    live.Target(),
		SkillID:     int(req.SkillID),
		Definitions: l.skills,
	})
	if err != nil {
		sendMagicCastFailure(live, started.Definition, err)
		return
	}
	def := started.Definition
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

	targetIDs := []int32{target.ObjectID()}
	controller.Schedule(plan, actorcast.Hooks{
		Launch: func() bool {
			if reason := actorcast.RevalidateLaunch(live.Character, target, def); reason != actorcast.LaunchAbortNone {
				sendLaunchAbort(live, reason)
				return false
			}
			l.broadcastLiveFrame(live, func() wire.Frame {
				return serverpackets.FrameMagicSkillLaunched(live.ObjectID(), int32(def.ID), int32(def.Level), targetIDs)
			})
			return true
		},
		Hit: func() {
			result := actorcast.ApplyEffectsResult(actorcast.EffectHandlers{Targets: l.targets, Skills: l.skillHandlers}, live.Character, target, def)
			sendSkillHandlerResult(live, result)
			if result.CubicAdded {
				l.broadcastCharacterInfo(live)
			}
			sendMagicStatusUpdate(live, beforeVitals)
		},
		Failed: func(err error) {
			sendMagicCastFailureReason(live, def, err)
			sendMagicStatusUpdate(live, beforeVitals)
		},
	})
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
// drives for an ordinary active skill. The on/off decision and effect
// application both happen inside actorcast.ApplyToggle; this handler only
// translates the outcome into packets.
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
	if err != nil {
		sendMagicCastFailure(live, def, err)
		return
	}

	selfObject := skillCastObject(live)
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameMagicSkillUse(selfObject, selfObject, int32(def.ID), int32(def.Level), 0, 0, false)
	})
	if activated {
		actorcast.ApplyEffects(handlers, live.Character, target, def)
	}
}

// broadcastCastAborted tells the caster and everyone watching it that an
// in-flight cast was cancelled: the cancel animation goes to the whole
// known list, while the action-failed acknowledgement is the caster's
// alone. interrupted additionally sends CASTING_INTERRUPTED to the caster
// alone, matching CreatureCast.interrupt() vs the unconditional stop().
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
	sendMagicActionFailed(live)
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
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughItems))
	case errors.Is(err, actorcast.ErrSkillDisabled):
		live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageS1PreparedForReuse, int32(def.ID), int32(def.Level)))
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

func sendSkillHandlerResult(live *livePlayer, result actorcast.EffectResult) {
	if live == nil {
		return
	}
	for i := 0; i < result.AttackFailed; i++ {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageAttackFailed))
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
