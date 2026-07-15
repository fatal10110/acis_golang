package network

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
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
	inst := inv.ItemByObjectID(req.ObjectID)
	if inst == nil {
		return
	}
	count := int(req.Count)
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok || !inst.Dropable(tmpl) || inst.QuestItem(tmpl) || inst.Count < count {
		return
	}
	if !tmpl.Stackable && count > 1 {
		return
	}
	if !dropInRange(live, int(req.X), int(req.Y), int(req.Z)) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotDiscardDistanceTooFar))
		return
	}

	newObjectID := int32(0)
	if inst.Count > count {
		if l.ids == nil {
			return
		}
		var err error
		newObjectID, err = l.ids.NextID()
		if err != nil {
			l.log.Error().Err(err).Msg("allocate dropped item id")
			return
		}
	}
	wasEquipped := inst.Equipped() && inst.Count <= count
	dropped := inv.DropItem(req.ObjectID, count, newObjectID)
	if dropped == nil {
		return
	}
	ground, err := grounditem.New(*dropped, tmpl)
	if err != nil {
		l.log.Error().Err(err).Msg("build dropped ground item")
		return
	}

	l.sendInventoryUpdate(live, inv)
	if wasEquipped {
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
	inst := inv.ItemByObjectID(objectID)
	if inst == nil {
		return
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok || !inst.Destroyable(tmpl) || tmpl.HeroItem() || inst.Count < count {
		return
	}
	if !tmpl.Stackable && count > 1 {
		return
	}

	wasEquipped := inst.Equipped() && inst.Count <= count
	if inv.DestroyItem(inst, count) == nil {
		return
	}
	l.sendInventoryUpdate(live, inv)
	if wasEquipped {
		l.broadcastEquipmentChange(live)
	}
}

func (l *GameClientLink) crystallizeLiveItem(live *livePlayer, req clientpackets.RequestCrystallizeItem) {
	if !liveItemOpsAllowed(live) || req.Count <= 0 {
		return
	}
	skillLevel := live.SkillLevel(crystallizeSkillID)
	if skillLevel <= 0 {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCrystallizeLevelTooLow))
		return
	}

	inv := live.Inventory()
	if inv == nil {
		return
	}
	inst := inv.ItemByObjectID(req.ObjectID)
	if inst == nil {
		return
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok || tmpl.HeroItem() || inst.ShadowItem(tmpl) {
		return
	}
	crystalItemID, crystalCount, ok := tmpl.CrystalReward(inst.EnchantLevel)
	if !ok {
		return
	}
	if !item.CanCrystallize(tmpl.Crystal, skillLevel) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCrystallizeLevelTooLow))
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if _, ok := inv.Templates().Get(crystalItemID); !ok || l.ids == nil {
		return
	}
	crystalObjectID, err := l.ids.NextID()
	if err != nil {
		l.log.Error().Err(err).Msg("allocate crystal item id")
		return
	}

	count := int(req.Count)
	if count > inst.Count {
		count = inst.Count
	}
	wasEquipped := inst.Equipped() && inst.Count <= count
	sourceItemID := inst.TemplateID
	if inv.DestroyItem(inst, count) == nil {
		return
	}
	if inv.AddNew(crystalItemID, int(crystalCount), crystalObjectID) == nil {
		return
	}

	live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageItemCrystallized, sourceItemID))
	l.sendInventoryUpdate(live, inv)
	if wasEquipped {
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
