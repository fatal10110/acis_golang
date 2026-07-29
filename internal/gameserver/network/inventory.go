package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func (l *GameClientLink) useItem(live *livePlayer, objectID int32) {
	if live == nil {
		return
	}
	if live.Operating() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageItemsUnavailableForStore))
		return
	}
	if l.trades != nil && l.trades.HasActive(live.ObjectID()) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotPickupOrUseItemTrading))
		return
	}
	inv := live.Inventory()
	if inv == nil {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	inst := inv.ItemByObjectID(objectID)
	if inst == nil {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if live.ItemDisabled(objectID) {
		l.rejectDisabledItemUse(live, tmpl)
		return
	}
	if inst.QuestItem(tmpl) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotUseQuestItems))
		return
	}
	if !liveItemOpsAllowed(live) {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if !l.playerConfig.KarmaPlayerCanTeleport && live.Karma() > 0 && itemBlockedByKarmaTeleport(tmpl, l.skills) {
		return
	}
	if live.Fishing() && tmpl.DefaultAction != item.ActionFishingShot {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotDoWhileFishing))
		return
	}
	if !inst.Equipped() && rejectUseItemConditions(live, tmpl) {
		return
	}
	if l.useEnchantScroll(live, inst) {
		return
	}
	if l.useConsumableSkillItem(live, inv, inst) {
		return
	}
	if l.useItemAICast(live, inv, inst) {
		return
	}
	if l.useShotItem(live, inv, inst) {
		return
	}
	if l.useBeastShotItem(live, inv, inst) {
		return
	}
	res, ok := l.inventoryService().ToggleEquipItem(inv, objectID)
	if !ok {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	l.applyEquipStatChanges(live, inv, res)
	l.sendInventoryUpdate(live, inv)
	l.broadcastEquipmentChange(live)
}

// applyEquipStatChanges attaches or detaches the stat functions each
// instance in res.Changed contributes while equipped — item-attached
// passive skills and equip modifiers — based on its current equip state.
// Unequip and equip changes for the same paperdoll slot arrive in that
// order (the old occupant first, the new one second), mirroring the
// reference's unequip-before-equip listener sequencing for one slot swap.
func (l *GameClientLink) applyEquipStatChanges(live *livePlayer, inv *itemcontainer.Inventory, res invops.Result) {
	if l.skills == nil || live == nil || inv == nil {
		return
	}
	for _, inst := range res.Changed {
		tmpl, ok := inv.Templates().Get(inst.TemplateID)
		if !ok {
			continue
		}
		if inst.Equipped() {
			if err := l.skills.EquipItemStats(live.Character, inst, tmpl); err != nil {
				l.log.Error().Err(err).Int32("object_id", inst.ObjectID).Msg("equip item stats")
			}
			continue
		}
		l.skills.UnequipItemStats(live.Character, inst, tmpl)
	}
}

func (l *GameClientLink) handleAutoSoulShot(live *livePlayer, req clientpackets.RequestAutoSoulShot) {
	if live == nil || live.AlikeDead() {
		return
	}
	inv := live.Inventory()
	if inv == nil {
		return
	}
	hasItem := inv.ItemByTemplateID(req.ItemID) != nil

	enabled := false
	switch req.Type {
	case 1:
		enabled = true
	case 0:
	default:
		return
	}

	switch live.ToggleAutoSoulShot(req.ItemID, enabled, hasItem, l.hasActiveSummon(live)) {
	case player.AutoSoulShotToggled:
	case player.AutoSoulShotNeedsSummon:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoServitorCannotAutomateUse))
		return
	default:
		return
	}
	live.SendFrame(serverpackets.FrameExAutoSoulShot(req.ItemID, enabled))
	if enabled {
		live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageUseOfItemWillBeAuto, req.ItemID))
		return
	}
	live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageAutoUseOfItemCancelled, req.ItemID))
}

func (l *GameClientLink) hasActiveSummon(live *livePlayer) bool {
	if l.world == nil || live == nil {
		return false
	}
	_, ok := l.world.Summon(live.ObjectID())
	return ok
}

// activeSummonTarget returns live's active pet or servitor as an
// actorcast.Target, or nil if it has none or doesn't expose that surface.
func (l *GameClientLink) activeSummonTarget(live *livePlayer) actorcast.Target {
	if l.world == nil || live == nil {
		return nil
	}
	obj, ok := l.world.Summon(live.ObjectID())
	if !ok {
		return nil
	}
	target, ok := obj.(actorcast.Target)
	if !ok {
		return nil
	}
	return target
}

// unequipItem clears whatever item occupies the paperdoll position that
// bodySlot (a Slot bitmask value from the item's own template) resolves
// to. An empty or unresolvable slot is a silent no-op.
func (l *GameClientLink) unequipItem(live *livePlayer, bodySlot int32) {
	if !liveItemOpsAllowed(live) {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	inv := live.Inventory()
	if inv == nil {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	res, ok := l.inventoryService().UnequipBodySlot(inv, bodySlot)
	if !ok {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	l.applyEquipStatChanges(live, inv, res)
	l.sendInventoryUpdate(live, inv)
	l.broadcastEquipmentChange(live)
}

func (l *GameClientLink) dropLiveItem(live *livePlayer, req clientpackets.RequestDropItem) {
	if !liveItemOpsAllowed(live) || l.groundItems == nil || req.Count <= 0 {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	inv := live.Inventory()
	if inv == nil {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	count := int(req.Count)
	if !dropInRange(live, int(req.X), int(req.Y), int(req.Z)) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotDiscardDistanceTooFar))
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}

	res, ok, err := l.inventoryService().DropItem(inv, req.ObjectID, count)
	if err != nil {
		l.log.Error().Err(err).Msg("allocate dropped item id")
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if !ok {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	ground, err := grounditem.New(*res.Dropped, res.Template)
	if err != nil {
		l.log.Error().Err(err).Msg("build dropped ground item")
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}

	l.sendInventoryUpdate(live, inv)
	if res.EquipmentChanged {
		l.broadcastEquipmentChange(live)
	}

	l.groundItems.Drop(ground, task.DropOptions{
		X:             int(req.X),
		Y:             int(req.Y),
		Z:             int(req.Z),
		Heading:       live.CurrentHeading(),
		PlayerDropped: true,
		DropperID:     live.ObjectID(),
	})
}

func (l *GameClientLink) destroyLiveItem(live *livePlayer, objectID int32, count int) {
	if !liveItemOpsAllowed(live) || count <= 0 {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	inv := live.Inventory()
	if inv == nil {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	res, ok := l.inventoryService().DestroyItem(inv, objectID, count)
	if !ok {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	l.sendInventoryUpdate(live, inv)
	if res.EquipmentChanged {
		l.broadcastEquipmentChange(live)
	}
}

func (l *GameClientLink) crystallizeLiveItem(live *livePlayer, req clientpackets.RequestCrystallizeItem) {
	if !liveItemOpsAllowed(live) || req.Count <= 0 {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	inv := live.Inventory()
	res, failure, err := l.inventoryService().CrystallizeItem(inv, req.ObjectID, int(req.Count), live.SkillLevel(crystallizeSkillID))
	if err != nil {
		l.log.Error().Err(err).Msg("allocate crystal item id")
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	switch failure {
	case invops.CrystallizeOK:
	case invops.CrystallizeNoSkill:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCrystallizeLevelTooLow))
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	case invops.CrystallizeGradeTooHigh:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCrystallizeLevelTooLow))
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	default:
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}

	live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageItemCrystallized, res.SourceItemID))
	l.sendInventoryUpdate(live, inv)
	if res.EquipmentChanged {
		l.broadcastEquipmentChange(live)
	}
}

func (l *GameClientLink) sendInventoryUpdate(live *livePlayer, inv *itemcontainer.Inventory) {
	updates := inv.DrainUpdates()
	if len(updates) == 0 {
		return
	}
	items := inv.Items()
	frame, err := serverpackets.FrameInventoryUpdate(updates, items, inv.Templates())
	if err != nil {
		l.log.Error().Err(err).Msg("build InventoryUpdate")
		return
	}
	live.SendFrame(frame)
}

// broadcastEquipmentChange resends UserInfo to live (refreshing its own
// paperdoll/stats) and CharInfo to every client that already knows about
// it (refreshing the worn-item visuals on their screen).
func (l *GameClientLink) broadcastEquipmentChange(live *livePlayer) {
	l.broadcastCharacterInfo(live)
}

// broadcastCharacterInfo resends UserInfo to live (refreshing its own
// visible state) and CharInfo to every client that already knows about it,
// matching the reference's broadcastUserInfo().
func (l *GameClientLink) broadcastCharacterInfo(live *livePlayer) {
	items := live.inventoryItems()
	live.SendFrame(serverpackets.FrameUserInfo(serverpackets.UserInfoSnapshot{
		Character: live.Character, Template: live.template, Items: items,
	}))
	if l.world == nil {
		return
	}
	info := serverpackets.CharInfoSnapshot{Character: live.Character, Template: live.template, Items: items}
	broadcastFrame(func() wire.Frame {
		return serverpackets.FrameCharInfo(info)
	}, func(send func(frameReceiver)) {
		l.world.ForEachKnown(live, func(o world.Tracked) {
			if receiver, ok := o.(frameReceiver); ok {
				send(receiver)
			}
		})
	})
}

// liveItemOpsAllowed reports whether live may currently use, equip, or
// unequip an item: not gone, not dead, and free of every crowd-control
// state that locks item interaction (stunned, sleeping, paralyzed, or
// afraid).
func liveItemOpsAllowed(live *livePlayer) bool {
	return live != nil && !live.AlikeDead() && !live.Stunned() && !live.Sleeping() && !live.Paralyzed() && !live.Afraid()
}

func dropInRange(live *livePlayer, x, y, z int) bool {
	sx, sy, sz := live.Position()
	return location.In3DRange(sx, sy, sz, x, y, z, dropInteractionDistance)
}
