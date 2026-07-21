package network

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	itemhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/item"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// useItemAICast runs an item-carried skill that isn't an instant-cast
// potion (a regular scroll etc.) through the same Start/Hit/Finish cast
// sequence a player-initiated RequestMagicSkillUse drives, targeting the
// player's current selection. The item is consumed once the cast starts
// (matching how the reference commits an item-driven cast's consumption
// up front, before the hit/effect phase), not only on a successful hit.
//
// It reports whether inst was handled by this path, so the caller's
// equip-toggle fallback still answers the client for anything else.
func (l *GameClientLink) useItemAICast(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance) bool {
	if live == nil || inv == nil || inst == nil {
		return false
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok {
		return false
	}
	def, ok := itemhandler.ResolveAICastSkill(tmpl, l.skills)
	if !ok {
		return false
	}

	beforeVitals := live.Vitals()
	controller := live.castController()
	started, err := actorcast.StartItemSkill(actorcast.ItemSkillRequest{
		Now:         time.Now(),
		Controller:  controller,
		Caster:      live.Character,
		Selected:    live.target,
		Skill:       modelskill.Ref{ID: def.ID, Level: def.Level},
		Definitions: l.skills,
	})
	if err != nil {
		sendMagicCastFailure(live, started.Definition, err)
		return true
	}
	target := started.Target
	plan := started.Plan

	if _, ok := l.inventoryService().DestroyItem(inv, inst.ObjectID, 1); !ok {
		sendMagicCastFailure(live, def, actorcast.ErrNotEnoughItems)
		controller.Stop()
		return true
	}
	l.sendInventoryUpdate(live, inv)

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
		return true
	}
	actorcast.ApplyEffects(actorcast.EffectHandlers{Targets: l.targets, Skills: l.skillHandlers}, live.Character, target, def)
	sendMagicStatusUpdate(live, beforeVitals)
	controller.Finish()
	return true
}
