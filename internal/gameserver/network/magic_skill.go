package network

import (
	"errors"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
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

	if def, ok := l.skills.Definition(modelskill.Ref{ID: modelskill.ID(req.SkillID), Level: live.SkillLevel(int(req.SkillID))}); ok && def.Activation == modelskill.ActivationToggle {
		l.handleToggleSkillUse(live, req)
		return
	}

	beforeVitals := live.Vitals()
	controller := live.castController()
	started, err := actorcast.StartPlayerSkill(actorcast.PlayerSkillRequest{
		Now:         time.Now(),
		Controller:  controller,
		Caster:      live.Character,
		Selected:    live.target,
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
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameMagicSkillLaunched(live.ObjectID(), int32(def.ID), int32(def.Level), targetIDs)
	})

	if err := controller.Hit(); err != nil {
		sendMagicCastFailure(live, def, err)
		sendMagicStatusUpdate(live, beforeVitals)
		controller.Stop()
		return
	}
	l.applySkillEffects(live, target, def)
	sendMagicStatusUpdate(live, beforeVitals)
	controller.Finish()
}

// handleToggleSkillUse applies casting a toggle skill: an already-active
// instance turns off at no cost, an inactive one pays its MP/HP cost and
// turns on. A toggle's cast window is instantaneous — there is no cast bar,
// no launch packet, and activating one never installs a reuse delay — so
// this bypasses the timed Start/Hit/Finish sequence handleMagicSkillUse
// drives for an ordinary active skill.
func (l *GameClientLink) handleToggleSkillUse(live *livePlayer, req clientpackets.RequestMagicSkillUse) {
	def, target, err := actorcast.ResolvePlayerToggle(actorcast.PlayerToggleRequest{
		Caster:      live.Character,
		Selected:    live.target,
		SkillID:     int(req.SkillID),
		Definitions: l.skills,
	})
	if err != nil {
		sendMagicCastFailure(live, def, err)
		return
	}

	selfObject := skillCastObject(live)
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameMagicSkillUse(selfObject, selfObject, int32(def.ID), int32(def.Level), 0, 0, false)
	})

	alreadyActive := handlerskill.ActiveEffect(live.Character, def.ID)
	activated, err := live.castController().CastToggle(alreadyActive, def)
	if err != nil {
		sendMagicCastFailure(live, def, err)
		return
	}

	if alreadyActive {
		handlerskill.StopEffect(live.Character, def.ID)
		return
	}
	if activated {
		l.applySkillEffects(live, target, def)
	}
}

// applySkillEffects resolves def's affected target set from resolved (the
// already cast-validated single selection) and applies the skill's
// effects to it. A caster or resolved target that doesn't satisfy the
// target-resolution or effect-application surfaces is skipped rather than
// failing the cast — the same graceful degradation the effect handlers
// already use for actor state this port hasn't modeled yet.
func (l *GameClientLink) applySkillEffects(live *livePlayer, resolved actorcast.Target, def modelskill.Definition) {
	caster, ok := any(live.Character).(skilltarget.Creature)
	if !ok {
		return
	}
	selected, _ := resolved.(skilltarget.Creature)

	handler, ok := l.targets.Handler(def.Target)
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

	l.skillHandlers.Use(handlerskill.Cast{
		Caster:  live.Character,
		Skill:   def,
		Targets: castTargets,
	})
}

func skillCastObject(obj actorcast.Target) serverpackets.SkillCastObject {
	x, y, z := obj.Position()
	return serverpackets.SkillCastObject{
		ObjectID: obj.ObjectID(),
		Location: location.Location{X: x, Y: y, Z: z},
	}
}

func sendMagicCastFailure(live *livePlayer, def modelskill.Definition, err error) {
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
	}
	sendMagicActionFailed(live)
}

func sendMagicActionFailed(live *livePlayer) {
	if live != nil {
		live.SendFrame(serverpackets.FrameActionFailed())
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
