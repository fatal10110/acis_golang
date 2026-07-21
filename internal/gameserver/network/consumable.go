package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	itemhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/item"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// useConsumableSkillItem runs the instant-cast (potion) item-use path and
// maps its outcome to client packets: InventoryUpdate for the consumed
// stack, broadcast MagicSkillUse, USE_S1, and a StatusUpdate when the
// skill changed HP or MP. A reuse rejection or consume failure maps to
// the same cast-failure reply a player skill cast produces.
//
// It reports whether inst was handled by this path. A non-consumable, an
// item whose carried skill isn't an instant-cast potion, or an herb
// returns false so the caller's equip-toggle fallback still answers the
// client.
func (l *GameClientLink) useConsumableSkillItem(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance) bool {
	if live == nil || inv == nil || inst == nil {
		return false
	}
	beforeVitals := live.Vitals()
	res := itemhandler.Use(itemhandler.UseRequest{
		Caster:      live.Character,
		Inventory:   inv,
		Item:        inst,
		Definitions: l.skills,
		Effects:     actorcast.EffectHandlers{Targets: l.targets, Skills: l.skillHandlers},
		Destroyer:   l.inventoryService(),
	})
	switch res.Outcome {
	case itemhandler.NotHandled:
		return false
	case itemhandler.PetRejected:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageItemNotForPets))
		return true
	case itemhandler.ReuseRejected:
		sendMagicCastFailure(live, res.Skill, actorcast.ErrSkillDisabled)
		return true
	case itemhandler.NotEnoughItems:
		sendMagicCastFailure(live, res.Skill, actorcast.ErrNotEnoughItems)
		return true
	case itemhandler.Applied:
		l.sendInventoryUpdate(live, inv)
		self := skillCastObject(live)
		l.broadcastLiveFrame(live, func() wire.Frame {
			return serverpackets.FrameMagicSkillUse(self, self, int32(res.Skill.ID), int32(res.Skill.Level), 0, 0, false)
		})
		live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageUseS1, int32(res.Skill.ID), int32(res.Skill.Level)))
		sendMagicStatusUpdate(live, beforeVitals)
		return true
	}
	return false
}
