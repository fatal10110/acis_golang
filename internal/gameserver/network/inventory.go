package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func (l *GameClientLink) useItem(live *livePlayer, objectID int32) {
	if !liveItemOpsAllowed(live) {
		live.SendFrame(serverpackets.FrameActionFailed())
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
	if _, ok := inv.Templates().Get(inst.TemplateID); !ok {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if l.useEnchantScroll(live, inst) {
		return
	}
	if _, ok := l.inventoryService().ToggleEquipItem(inv, objectID); !ok {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	l.sendInventoryUpdate(live, inv)
	l.broadcastEquipmentChange(live)
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
	if _, ok := l.inventoryService().UnequipBodySlot(inv, bodySlot); !ok {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
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
	items := live.inventoryItems()
	live.SendFrame(serverpackets.FrameUserInfo(serverpackets.UserInfoSnapshot{
		Character: live.Character, Template: live.template, Items: items,
	}))
	if l.world == nil {
		return
	}
	l.world.ForEachKnown(live, func(o world.Tracked) {
		receiver, ok := o.(interface{ SendFrame(wire.Frame) bool })
		if !ok {
			return
		}
		receiver.SendFrame(serverpackets.FrameCharInfo(serverpackets.CharInfoSnapshot{
			Character: live.Character, Template: live.template, Items: items,
		}))
	})
}

func liveItemOpsAllowed(live *livePlayer) bool {
	return live != nil && !live.AlikeDead()
}

func dropInRange(live *livePlayer, x, y, z int) bool {
	sx, sy, sz := live.Position()
	return location.In3DRange(sx, sy, sz, x, y, z, dropInteractionDistance)
}
