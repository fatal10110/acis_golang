package network

import (
	"context"
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func (l *GameClientLink) activePet(live *livePlayer) (*summon.Actor, *itemcontainer.Inventory, bool) {
	if live == nil || l.world == nil {
		return nil, nil, false
	}
	obj, ok := l.world.Summon(live.ObjectID())
	if !ok {
		return nil, nil, false
	}
	pet, ok := obj.(*summon.Actor)
	if !ok || !pet.IsPet() {
		return nil, nil, false
	}
	inv := pet.PetInventory()
	return pet, inv, inv != nil
}

func (l *GameClientLink) giveItemToPet(ctx context.Context, live *livePlayer, req clientpackets.RequestGiveItemToPet) {
	if req.Count <= 0 || !liveItemOpsAllowed(live) {
		return
	}
	pet, petInv, ok := l.activePet(live)
	if !ok {
		return
	}
	playerInv := live.Inventory()
	if playerInv == nil {
		return
	}
	inst := playerInv.ItemByObjectID(req.ObjectID)
	if inst == nil || inst.Augmented() {
		return
	}
	tmpl, ok := playerInv.Templates().Get(inst.TemplateID)
	if !ok {
		return
	}
	if itemForbiddenForPet(inst, tmpl) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageItemNotForPets))
		return
	}
	if pet.Dead() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotGiveItemsToDeadPet))
		return
	}
	if !withinInteractionDistance(live, pet) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetTooFar))
		return
	}
	if !petInv.ValidateCapacity(petInv.SlotsNeededFor(inst, tmpl)) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessagePetCannotCarryMoreItems))
		return
	}
	if !petInv.ValidateWeight(int(tmpl.Weight) * int(req.Count)) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessagePetTooEncumbered))
		return
	}

	cancelActiveEnchant(live)
	l.transferInventoryItem(ctx, live, playerInv, petInv, req.ObjectID, int(req.Count), true)
}

func (l *GameClientLink) getItemFromPet(ctx context.Context, live *livePlayer, req clientpackets.RequestGetItemFromPet) {
	if req.Count <= 0 || live == nil {
		return
	}
	_, petInv, ok := l.activePet(live)
	if !ok {
		return
	}
	playerInv := live.Inventory()
	if playerInv == nil {
		return
	}
	cancelActiveEnchant(live)
	l.transferInventoryItem(ctx, live, petInv, playerInv, req.ObjectID, int(req.Count), false)
}

func (l *GameClientLink) petUseItem(ctx context.Context, live *livePlayer, req clientpackets.RequestPetUseItem) {
	pet, petInv, ok := l.activePet(live)
	if !ok {
		return
	}
	inst := petInv.ItemByObjectID(req.ObjectID)
	if inst == nil {
		return
	}
	tmpl, ok := petInv.Templates().Get(inst.TemplateID)
	if !ok {
		return
	}
	if live == nil || live.AlikeDead() || pet.Dead() {
		live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageS1CannotBeUsed, inst.TemplateID))
		return
	}
	if !petItem(tmpl) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessagePetCannotUseItem))
		return
	}
	if !petCanWear(pet, tmpl) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessagePetCannotUseItem))
		return
	}

	if inst.Equipped() {
		if old := petInv.UnequipSlot(inst.LocationData); old != nil {
			persistUpdate(ctx, l, old)
			live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessagePetTookOffS1, old.TemplateID))
			l.sendPetInventoryUpdate(live, petInv)
		}
		return
	}

	slot := itemcontainer.Chest
	if tmpl.Weapon != nil && tmpl.Weapon.Type == item.WeaponPet {
		slot = itemcontainer.RHand
	}
	old := petInv.SetPaperdollItem(slot, inst, tmpl)
	if old != nil {
		persistUpdate(ctx, l, old)
	}
	persistUpdate(ctx, l, inst)
	live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessagePetPutOnS1, inst.TemplateID))
	l.sendPetInventoryUpdate(live, petInv)
}

func (l *GameClientLink) transferInventoryItem(ctx context.Context, live *livePlayer, from, to *itemcontainer.Inventory, objectID int32, count int, toPet bool) {
	source := from.ItemByObjectID(objectID)
	if source == nil {
		return
	}
	tmpl, ok := from.Templates().Get(source.TemplateID)
	if !ok {
		return
	}

	newObjectID := int32(0)
	if source.Count > count {
		targetStack := (*item.Instance)(nil)
		if tmpl.Stackable {
			targetStack = to.ItemByTemplateID(source.TemplateID)
		}
		if targetStack == nil {
			if l.ids == nil {
				return
			}
			id, err := l.ids.NextID()
			if err != nil {
				l.log.Error().Err(err).Msg("allocate transfer item id")
				return
			}
			newObjectID = id
		}
	}

	result, freedObjectID, freed := from.TransferItem(objectID, count, to, newObjectID)
	if result == nil {
		return
	}
	if remaining := from.ItemByObjectID(objectID); remaining != nil {
		persistUpdate(ctx, l, remaining)
	}
	if freed {
		persistDelete(ctx, l, freedObjectID)
	}
	if result.ObjectID == objectID || to.ItemByObjectID(result.ObjectID) == result && newObjectID == 0 {
		persistUpdate(ctx, l, result)
	} else {
		persistSave(ctx, l, result)
	}

	if toPet {
		l.sendInventoryUpdate(live, from)
		l.sendPetInventoryUpdate(live, to)
		return
	}
	l.sendPetInventoryUpdate(live, from)
	l.sendInventoryUpdate(live, to)
}

func itemForbiddenForPet(inst *item.Instance, tmpl *item.Template) bool {
	if tmpl.HeroItem() || !inst.Dropable(tmpl) || !inst.Destroyable(tmpl) || !inst.Tradable(tmpl) {
		return true
	}
	return tmpl.EtcItem != nil && (tmpl.EtcItem.Type == item.EtcItemArrow || tmpl.EtcItem.Type == item.EtcItemShot)
}

func petItem(tmpl *item.Template) bool {
	return (tmpl.Weapon != nil && tmpl.Weapon.Type == item.WeaponPet) || (tmpl.Armor != nil && tmpl.Armor.Type == item.ArmorPet)
}

func petCanWear(pet *summon.Actor, tmpl *item.Template) bool {
	switch pet.NPCID() {
	case 12311, 12312, 12313:
		return tmpl.Slot == item.SlotHatchling
	case 12077:
		return tmpl.Slot == item.SlotWolf
	case 12526, 12527, 12528:
		return tmpl.Slot == item.SlotStrider
	case 12780, 12781, 12782:
		return tmpl.Slot == item.SlotBabyPet
	default:
		return false
	}
}

func withinInteractionDistance(live *livePlayer, pet *summon.Actor) bool {
	ax, ay, az := live.Position()
	bx, by, bz := pet.Position()
	dx := float64(ax - bx)
	dy := float64(ay - by)
	dz := float64(az - bz)
	return math.Sqrt(dx*dx+dy*dy+dz*dz) <= dropInteractionDistance
}

func (l *GameClientLink) sendPetInventoryUpdate(live *livePlayer, inv *itemcontainer.Inventory) {
	updates := inv.DrainUpdates()
	if len(updates) == 0 {
		return
	}
	frame, err := serverpackets.FramePetInventoryUpdate(updates, inv.Items(), inv.Templates())
	if err != nil {
		l.log.Error().Err(err).Msg("build PetInventoryUpdate")
		return
	}
	live.SendFrame(frame)
}
