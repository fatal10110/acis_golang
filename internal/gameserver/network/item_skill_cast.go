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
// player's current selection. Starting the cast, consuming the item, and
// applying its effects are itemhandler's decisions (itemhandler.
// ResolveAICastSkill / actorcast.StartItemSkill / itemhandler.
// ConsumeAICastItem / itemhandler.CompleteAICast); this method only builds
// and sends the packets those decisions produce, in the order the
// reference itself uses: the item is consumed before the cast/launch
// packets go out, not only on a successful hit.
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

	consumed := itemhandler.ConsumeAICastItem(itemhandler.ConsumeAICastItemRequest{
		Controller: controller,
		Definition: def,
		Inventory:  inv,
		Item:       inst,
		Template:   tmpl,
		Destroyer:  l.inventoryService(),
	})
	if consumed.Err != nil {
		sendMagicCastFailure(live, def, consumed.Err)
		return true
	}
	l.sendInventoryUpdate(live, inv)
	if consumed.SharedReuseGroup >= 0 {
		live.SendFrame(serverpackets.FrameExUseSharedGroupItem(inst.TemplateID, consumed.SharedReuseGroup, consumed.ReuseMillis, consumed.ReuseMillis))
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

	result := itemhandler.CompleteAICast(itemhandler.CompleteAICastRequest{
		Controller: controller,
		Definition: def,
		Caster:     live.Character,
		Target:     target,
		Effects:    actorcast.EffectHandlers{Targets: l.targets, Skills: l.skillHandlers},
	})
	if result.Err != nil {
		sendMagicCastFailure(live, def, result.Err)
		sendMagicStatusUpdate(live, beforeVitals)
		return true
	}
	sendMagicStatusUpdate(live, beforeVitals)
	return true
}
