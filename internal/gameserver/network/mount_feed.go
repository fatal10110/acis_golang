package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// mountFeedTable resolves a mount's feeding data from its NPC template's
// pet data row for the rider's level.
type mountFeedTable struct{ npcs *npc.Table }

func (t mountFeedTable) MountFeed(npcID int32, level int) (player.MountFeed, bool) {
	tpl, ok := t.npcs.Get(int(npcID))
	if !ok || tpl.Pet == nil {
		return player.MountFeed{}, false
	}
	row, ok := tpl.Pet.Levels[level]
	if !ok {
		return player.MountFeed{}, false
	}
	return player.MountFeed{
		MaxMeal:       row.MaxMeal,
		MealInNormal:  row.MountMealInNormal,
		MealInBattle:  row.MealInBattle,
		Food1:         int32(tpl.Pet.Food1),
		Food2:         int32(tpl.Pet.Food2),
		AutoFeedLimit: tpl.Pet.AutoFeedLimit,
	}, true
}

// feedMountFood has a hungry mount eat one unit of the rider's food item
// objectID, through the food's item handler (eatPetFood). The rider is told
// the mount ate even when the item gave nothing.
func (l *GameClientLink) feedMountFood(live *livePlayer, objectID int32) {
	inv := live.Inventory()
	if inv == nil {
		return
	}
	inst := inv.ItemByObjectID(objectID)
	if inst == nil {
		return
	}
	templateID := inst.TemplateID
	if !l.eatPetFood(live, inv, inst) {
		return
	}
	live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessagePetTookS1BecauseHeWasHungry, templateID))
}

// eatPetFood is a player's own use of a pet-food item, and reports whether
// inst is one. A rider whose mount eats that food uses up one unit: the feed
// skill's effect plays for everyone around, and the gauge rises by the
// skill's feed value scaled by the pet food rate. Anyone else is told the
// item cannot be used. A food item with no feed skill is ignored with no
// packet, as the reference does; a use-item request leaves no client action
// pending.
func (l *GameClientLink) eatPetFood(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance) bool {
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok || tmpl.EtcItem == nil || tmpl.EtcItem.Handler != petFoodsHandler {
		return false
	}
	templateID := inst.TemplateID
	if l.skills == nil || l.inventory == nil {
		return true
	}
	amount, ok := petFoodFeedAmount(l.skills, l.petConfig.FoodRate, templateID)
	if !ok {
		return true
	}
	if !live.Character.MountEats(templateID) {
		live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageS1CannotBeUsed, templateID))
		return true
	}
	if _, ok := l.inventory.DestroyItem(inv, inst.ObjectID, 1); !ok {
		return true
	}
	magicID := petFoodMagicIDs[templateID]
	self := skillCastObject(live)
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameMagicSkillUse(self, self, magicID, 1, 0, 0, false)
	})
	live.Character.AddMountFeed(amount)
	return true
}

// broadcastDismount shows a rider getting off its mount: the feed gauge
// empties, the skill list refreshes, everyone around sees the dismount, and
// the rider's appearance and speed are resent.
func (l *GameClientLink) broadcastDismount(live *livePlayer) {
	live.SendFrame(serverpackets.FrameSetupGauge(serverpackets.GaugeGreen, 0, 0))
	live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameDismount(live.ObjectID())
	})
	l.broadcastCharacterInfo(live)
}

// throwStarvedRider tells a rider its mount left for lack of feed. A rider
// thrown from a flying mount is taken to the nearest town, sparing it the
// fall.
func (l *GameClientLink) throwStarvedRider(live *livePlayer, wasFlying bool) {
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageOutOfFeedMountCanceled))
	if !wasFlying {
		return
	}
	dest, ok := l.restartDestination(live)
	if !ok {
		l.log.Warn().Int32("object_id", live.ObjectID()).Msg("mount feed: no town restart point resolved")
		return
	}
	l.teleportLivePlayer(live, dest, restartTeleportOffset)
}
