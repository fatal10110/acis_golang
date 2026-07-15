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
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func (l *GameClientLink) handleMagicSkillUse(live *livePlayer, req clientpackets.RequestMagicSkillUse) {
	if live == nil || live.AlikeDead() || req.SkillID <= 0 {
		sendMagicActionFailed(live)
		return
	}

	level := live.SkillLevel(int(req.SkillID))
	if level <= 0 || l.skills == nil {
		sendMagicActionFailed(live)
		return
	}

	def, ok := l.skills.definition(modelskill.Ref{ID: modelskill.ID(req.SkillID), Level: level})
	if !ok || def.Activation != modelskill.ActivationActive {
		sendMagicActionFailed(live)
		return
	}

	target, ok := magicSkillTarget(live, def)
	if !ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
		sendMagicActionFailed(live)
		return
	}

	beforeHP, beforeMP := int(live.CurHP), int(live.CurMP)
	controller := live.castController()
	plan, err := controller.Start(time.Now(), target, def)
	if err != nil {
		sendMagicCastFailure(live, def, err)
		return
	}

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

type skillCastTarget interface {
	world.Tracked
	Position() (x, y, z int)
}

func magicSkillTarget(live *livePlayer, def modelskill.Definition) (skillCastTarget, bool) {
	switch def.Target {
	case modelskill.TargetNone, modelskill.TargetSelf, modelskill.TargetGround:
		return live, true
	case modelskill.TargetOne:
		if target, ok := live.target.(skillCastTarget); ok {
			return target, true
		}
	}
	return nil, false
}

func skillCastObject(obj skillCastTarget) serverpackets.SkillCastObject {
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

type liveCastActor struct {
	live *livePlayer
}

func (a liveCastActor) AttackSpeed(bool) int {
	if a.live == nil {
		return 1
	}
	return a.live.AttackSpeed()
}

func (a liveCastActor) ReuseRate(bool) float64 { return 1 }

func (a liveCastActor) MP() int {
	if a.live == nil {
		return 0
	}
	return int(a.live.CurMP)
}

func (a liveCastActor) HP() int {
	if a.live == nil {
		return 0
	}
	return int(a.live.CurHP)
}

func (liveCastActor) MPInitialCost(def modelskill.Definition) int { return def.MPInitialConsume }

func (liveCastActor) MPCost(def modelskill.Definition) int { return def.MPConsume }

func (a liveCastActor) ReduceMP(amount int) {
	if a.live == nil || amount <= 0 {
		return
	}
	a.live.CurMP = max(0, a.live.CurMP-float64(amount))
}

func (a liveCastActor) ReduceHP(amount int) {
	if a.live == nil || amount <= 0 {
		return
	}
	a.live.CurHP = max(0, a.live.CurHP-float64(amount))
}

func (a liveCastActor) SkillDisabled(key int32) bool {
	return a.live != nil && a.live.SkillDisabled(key)
}

func (a liveCastActor) DisableSkill(key int32, delay time.Duration) {
	if a.live != nil {
		a.live.DisableSkill(key, delay)
	}
}

func (a liveCastActor) AddSkillReuse(ref modelskill.Ref, key int32, delay time.Duration) {
	if a.live != nil {
		a.live.AddSkillReuse(ref, key, delay)
	}
}

func (liveCastActor) MagicMuted() bool { return false }

func (liveCastActor) PhysicalMuted() bool { return false }

func (liveCastActor) SpiritshotCharged() bool { return false }

func (liveCastActor) BlessedSpiritshotCharged() bool { return false }

func (liveCastActor) SkillMastery(modelskill.Definition) bool { return false }

func (a liveCastActor) ItemCount(itemID int) int {
	if a.live == nil || a.live.Inventory() == nil {
		return 0
	}
	return a.live.Inventory().ItemCount(int32(itemID), -1, true)
}

func (a liveCastActor) ConsumeItem(itemID, count int) bool {
	if a.live == nil || a.live.Inventory() == nil {
		return false
	}
	return a.live.Inventory().DestroyByTemplateID(int32(itemID), count) != nil
}
