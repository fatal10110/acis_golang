package network

import (
	"context"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/petitem"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
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
	res, failure, err := l.petItemService().GiveToPet(playerInv, petInv, pet, req.ObjectID, int(req.Count), withinInteractionDistance(live, pet))
	if err != nil {
		l.log.Error().Err(err).Msg("transfer item to pet")
		return
	}
	switch failure {
	case petitem.GiveNoop:
		return
	case petitem.GiveItemNotForPets:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageItemNotForPets))
		return
	case petitem.GiveDeadPet:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotGiveItemsToDeadPet))
		return
	case petitem.GiveTooFar:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetTooFar))
		return
	case petitem.GivePetCannotCarryMore:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessagePetCannotCarryMoreItems))
		return
	case petitem.GivePetTooEncumbered:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessagePetTooEncumbered))
		return
	}

	l.cancelActiveEnchant(live)
	l.applyPersistActions(ctx, res.Persist)
	l.sendInventoryUpdate(live, playerInv)
	l.sendPetInventoryUpdate(live, petInv)
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
	res, ok, err := l.petItemService().GetFromPet(petInv, playerInv, req.ObjectID, int(req.Count))
	if err != nil {
		l.log.Error().Err(err).Msg("transfer item from pet")
		return
	}
	if !ok {
		return
	}
	l.cancelActiveEnchant(live)
	l.applyPersistActions(ctx, res.Persist)
	l.sendPetInventoryUpdate(live, petInv)
	l.sendInventoryUpdate(live, playerInv)
}

func (l *GameClientLink) petGetItem(ctx context.Context, live *livePlayer, req clientpackets.RequestPetGetItem) {
	if live == nil || l.world == nil || l.groundItems == nil {
		return
	}
	pet, petInv, ok := l.activePet(live)
	if !ok {
		return
	}
	if !petitem.PickupAvailable(pet) {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	obj, ok := l.world.Object(req.ObjectID)
	if !ok {
		return
	}
	ground, ok := obj.(*grounditem.Item)
	if !ok || ground.Template == nil || ground.Count() <= 0 {
		return
	}

	result, failure := petitem.PickupGround(pet, petInv, ground)
	switch failure {
	case petitem.PickupOK:
	case petitem.PickupNoop:
		return
	case petitem.PickupPetUnavailable:
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	case petitem.PickupItemNotForPets:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageItemNotForPets))
		return
	case petitem.PickupPetCannotCarryMore:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessagePetCannotCarryMoreItems))
		return
	case petitem.PickupPetTooEncumbered:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessagePetTooEncumbered))
		return
	}

	l.broadcastGroundPickup(ground, pet.ObjectID())
	l.groundItems.Remove(ground)
	l.world.Despawn(ground)

	l.applyPersistActions(ctx, result.Persist)
	l.sendPetInventoryUpdate(live, petInv)
}

func (l *GameClientLink) petUseItem(ctx context.Context, live *livePlayer, req clientpackets.RequestPetUseItem) {
	pet, petInv, ok := l.activePet(live)
	if !ok {
		return
	}
	res, failure := petitem.UseItem(pet, petInv, req.ObjectID, live == nil || live.AlikeDead())
	switch failure {
	case petitem.UseNoop:
		return
	case petitem.UseCannotBeUsed:
		live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageS1CannotBeUsed, res.ItemID))
		return
	case petitem.UsePetCannotUseItem:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessagePetCannotUseItem))
		return
	}

	l.applyPersistActions(ctx, res.Persist)
	if res.Outcome == petitem.Unequipped {
		live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessagePetTookOffS1, res.ItemID))
		l.sendPetInventoryUpdate(live, petInv)
		return
	}
	live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessagePetPutOnS1, res.ItemID))
	l.sendPetInventoryUpdate(live, petInv)
}

func withinInteractionDistance(live *livePlayer, pet *summon.Actor) bool {
	ax, ay, az := live.Position()
	bx, by, bz := pet.Position()
	return location.In3DRange(ax, ay, az, bx, by, bz, dropInteractionDistance)
}

func (l *GameClientLink) broadcastGroundPickup(ground *grounditem.Item, pickerID int32) {
	if l.world == nil {
		return
	}
	l.world.ForEachKnown(ground, func(o world.Tracked) {
		receiver, ok := o.(interface{ SendFrame(wire.Frame) bool })
		if !ok {
			return
		}
		receiver.SendFrame(serverpackets.FrameGetItem(ground, pickerID))
	})
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
