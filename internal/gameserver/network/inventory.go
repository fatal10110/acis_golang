package network

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func (l *GameClientLink) useItem(live *livePlayer, objectID int32) {
	if !liveItemOpsAllowed(live) {
		return
	}
	inv := live.Inventory()
	if inv == nil {
		return
	}
	inst := inv.ItemByObjectID(objectID)
	if inst == nil {
		return
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok {
		return
	}
	if l.useEnchantScroll(live, inst) {
		return
	}
	if tmpl.Slot == item.SlotNone {
		return
	}

	var altered []*item.Instance
	if inst.Equipped() {
		if old := inv.UnequipSlot(inst.LocationData); old != nil {
			altered = []*item.Instance{old}
		}
	} else {
		altered = inv.EquipItem(inst, tmpl)
	}
	if len(altered) == 0 {
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
	if inv == nil || inv.ItemByTemplateID(req.ItemID) == nil {
		return
	}

	switch req.Type {
	case 1:
		if fishingShot(req.ItemID) {
			return
		}
		if summonShot(req.ItemID) {
			if l.world == nil {
				live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoServitorCannotAutomateUse))
				return
			}
			if _, hasSummon := l.world.Summon(live.ObjectID()); !hasSummon {
				live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoServitorCannotAutomateUse))
				return
			}
		}
		live.SetAutoSoulShot(req.ItemID, true)
		live.SendFrame(serverpackets.FrameExAutoSoulShot(req.ItemID, true))
		live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageUseOfItemWillBeAuto, req.ItemID))
	case 0:
		live.SetAutoSoulShot(req.ItemID, false)
		live.SendFrame(serverpackets.FrameExAutoSoulShot(req.ItemID, false))
		live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageAutoUseOfItemCancelled, req.ItemID))
	}
}

func fishingShot(itemID int32) bool {
	return itemID >= 6535 && itemID <= 6540
}

func summonShot(itemID int32) bool {
	return itemID >= 6645 && itemID <= 6647
}

// unequipItem clears whatever item occupies the paperdoll position that
// bodySlot (a Slot bitmask value from the item's own template) resolves
// to. An empty or unresolvable slot is a silent no-op.
func (l *GameClientLink) unequipItem(live *livePlayer, bodySlot int32) {
	if !liveItemOpsAllowed(live) {
		return
	}
	inv := live.Inventory()
	if inv == nil {
		return
	}
	paperdollSlot, ok := item.Slot(bodySlot).PaperdollIndex()
	if !ok {
		return
	}
	if inv.UnequipSlot(paperdollSlot) == nil {
		return
	}
	l.sendInventoryUpdate(live, inv)
	l.broadcastEquipmentChange(live)
}

func (l *GameClientLink) dropLiveItem(live *livePlayer, req clientpackets.RequestDropItem) {
	if !liveItemOpsAllowed(live) || l.groundItems == nil || req.Count <= 0 {
		return
	}
	inv := live.Inventory()
	if inv == nil {
		return
	}
	count := int(req.Count)
	if !dropInRange(live, int(req.X), int(req.Y), int(req.Z)) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotDiscardDistanceTooFar))
		return
	}

	res, ok, err := l.inventoryService().DropItem(inv, req.ObjectID, count)
	if err != nil {
		l.log.Error().Err(err).Msg("allocate dropped item id")
		return
	}
	if !ok {
		return
	}
	ground, err := grounditem.New(*res.Dropped, res.Template)
	if err != nil {
		l.log.Error().Err(err).Msg("build dropped ground item")
		return
	}

	l.sendInventoryUpdate(live, inv)
	if res.EquipmentChanged {
		l.broadcastEquipmentChange(live)
	}

	ground.SetDropperID(live.ObjectID())
	l.groundItems.Drop(ground, task.DropOptions{
		X:             int(req.X),
		Y:             int(req.Y),
		Z:             int(req.Z),
		Heading:       live.CurrentHeading(),
		PlayerDropped: true,
	})
	ground.SetDropperID(0)
}

func (l *GameClientLink) destroyLiveItem(live *livePlayer, objectID int32, count int) {
	if !liveItemOpsAllowed(live) || count <= 0 {
		return
	}
	inv := live.Inventory()
	if inv == nil {
		return
	}
	res, ok := l.inventoryService().DestroyItem(inv, objectID, count)
	if !ok {
		return
	}
	l.sendInventoryUpdate(live, inv)
	if res.EquipmentChanged {
		l.broadcastEquipmentChange(live)
	}
}

func (l *GameClientLink) crystallizeLiveItem(live *livePlayer, req clientpackets.RequestCrystallizeItem) {
	if !liveItemOpsAllowed(live) || req.Count <= 0 {
		return
	}
	inv := live.Inventory()
	res, failure, err := l.inventoryService().CrystallizeItem(inv, req.ObjectID, int(req.Count), live.SkillLevel(crystallizeSkillID))
	if err != nil {
		l.log.Error().Err(err).Msg("allocate crystal item id")
		return
	}
	switch failure {
	case invops.CrystallizeOK:
	case invops.CrystallizeNoSkill:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCrystallizeLevelTooLow))
		return
	case invops.CrystallizeGradeTooHigh:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCrystallizeLevelTooLow))
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	default:
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
	dx := float64(sx - x)
	dy := float64(sy - y)
	dz := float64(sz - z)
	return math.Sqrt(dx*dx+dy*dy+dz*dz) <= dropInteractionDistance
}
