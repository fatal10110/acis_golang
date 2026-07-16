package network

import (
	"errors"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
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

	beforeHP, beforeMP := int(live.CurHP), int(live.CurMP)
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
		sendMagicStatusUpdate(live, beforeHP, beforeMP)
		controller.Stop()
		return
	}
	sendMagicStatusUpdate(live, beforeHP, beforeMP)
	controller.Finish()
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

func sendMagicStatusUpdate(live *livePlayer, beforeHP, beforeMP int) {
	if live == nil {
		return
	}
	attrs := make([]serverpackets.StatusAttribute, 0, 2)
	if hp := int(live.CurHP); hp != beforeHP {
		attrs = append(attrs, serverpackets.StatusAttribute{Type: serverpackets.StatusCurrentHP, Value: hp})
	}
	if mp := int(live.CurMP); mp != beforeMP {
		attrs = append(attrs, serverpackets.StatusAttribute{Type: serverpackets.StatusCurrentMP, Value: mp})
	}
	if len(attrs) > 0 {
		live.SendFrame(serverpackets.FrameStatusUpdate(live.ObjectID(), attrs))
	}
}

func millis(d time.Duration) int {
	return int(d / time.Millisecond)
}
