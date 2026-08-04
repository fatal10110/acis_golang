package network

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/petitem"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// activePet is a pure lookup of live's currently spawned pet and its
// inventory. It does not register anything with the batching task: that
// happens once, structurally, wherever the pet becomes live for its owner
// (registerPetInventoryUpdates), the way character_flow.go wires the
// player's own inventory at spawn rather than on every lookup.
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
	if inv == nil {
		return nil, nil, false
	}
	return pet, inv, true
}

// registerPetInventoryUpdates registers pet's inventory with the batching
// task, matching the reference's Pet registering itself with
// InventoryUpdateTaskManager: the task is the only drainer, addressed to
// the owner's client. Call it once, when pet becomes live for live — from
// newPet, or wherever else a pet is attached to its owner.
func (l *GameClientLink) registerPetInventoryUpdates(pet *summon.Actor, live *livePlayer) {
	if l.inventoryUpdates != nil {
		wirePetInventoryUpdates(l.inventoryUpdates, pet, live, l.log)
	}
	// A pet's inventory persists through the same lazy task the owner's
	// does; its items carry the pet's own object id as owner.
	if l.itemInstances != nil && pet != nil {
		if inv := pet.PetInventory(); inv != nil {
			inv.SetItemPersister(l.itemInstances.Add)
		}
	}
}

// wirePetInventoryUpdates registers pet's inventory with updates, addressed
// to live's connection. Factored out of registerPetInventoryUpdates so test
// helpers that don't have a *GameClientLink handle can reach the same
// wiring against a task looked up another way.
func wirePetInventoryUpdates(updates *task.InventoryUpdates, pet *summon.Actor, live *livePlayer, log zerolog.Logger) {
	if updates == nil || pet == nil || live == nil {
		return
	}
	inv := pet.PetInventory()
	if inv == nil {
		return
	}
	owner := &petInventoryOwner{live: live, pet: pet, log: log}
	inv.SetUpdateNotifier(func() {
		updates.Add(inv, owner)
	})
}

// petInventoryOwner adapts a pet's inventory to task.InventoryUpdateOwner:
// visibility follows the pet itself, but delivery goes over the owning
// player's connection since pets have no connection of their own. Pets
// never teleport independently, so the reference's isTeleporting() gate is
// always false here.
type petInventoryOwner struct {
	live *livePlayer
	pet  *summon.Actor
	log  zerolog.Logger
}

func (o *petInventoryOwner) Visible() bool     { return o.pet.Visible() }
func (o *petInventoryOwner) Teleporting() bool { return false }

func (o *petInventoryOwner) SendInventoryUpdate(updates []itemcontainer.Update) {
	if len(updates) == 0 {
		return
	}
	inv := o.pet.PetInventory()
	if inv == nil {
		return
	}
	frame, err := serverpackets.FramePetInventoryUpdate(updates, inv.Items(), inv.Templates())
	if err != nil {
		o.log.Error().Err(err).Msg("build PetInventoryUpdate")
		return
	}
	o.live.SendFrame(frame)
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
	switch failure := l.petItems.CheckGive(playerInv, petInv, pet, live, req.ObjectID, int(req.Count)); failure {
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

	// Cancel before the mutation, matching the reference's
	// Player.cancelActiveEnchant() call placed right before
	// Player.transferItem(): a downstream transfer failure must not
	// leave a stale enchant selection active.
	l.cancelActiveEnchant(live)

	res, ok, err := l.petItems.Transfer(playerInv, petInv, req.ObjectID, int(req.Count))
	if err != nil {
		l.log.Error().Err(err).Msg("transfer item to pet")
		return
	}
	if !ok {
		return
	}
	l.applyPersistActions(ctx, res.Persist)
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

	// Cancel unconditionally before attempting the transfer, matching the
	// reference's Player.cancelActiveEnchant() call which precedes
	// Pet.transferItem() regardless of whether objectID resolves or the
	// transfer subsequently fails.
	l.cancelActiveEnchant(live)

	res, ok, err := l.petItems.GetFromPet(petInv, playerInv, req.ObjectID, int(req.Count))
	if err != nil {
		l.log.Error().Err(err).Msg("transfer item from pet")
		return
	}
	if !ok {
		return
	}
	l.applyPersistActions(ctx, res.Persist)
}

// petGetItem handles a command-your-pet-to-loot request. Once live is known
// non-nil (there is a connected client to answer), every rejection answers
// with ActionFailed so the command always resolves — the same "accepted
// action packet must never go unanswered" rule the player's own pickup path
// follows, since this opcode's client-side command state waits on a
// response exactly the same way.
func (l *GameClientLink) petGetItem(ctx context.Context, live *livePlayer, req clientpackets.RequestPetGetItem) {
	if live == nil {
		return
	}
	if l.world == nil || l.groundItems == nil {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	pet, petInv, ok := l.activePet(live)
	if !ok {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if !petitem.PickupAvailable(pet) {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	obj, ok := l.world.Object(req.ObjectID)
	if !ok {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	ground, ok := obj.(*grounditem.Item)
	if !ok || ground.Template == nil || ground.Count() <= 0 {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}

	result, failure := petitem.PickupGround(pet, petInv, ground)
	switch failure {
	case petitem.PickupOK:
	case petitem.PickupPetUnavailable:
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	case petitem.PickupItemNotForPets:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageItemNotForPets))
		return
	case petitem.PickupPetCannotCarryMore:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessagePetCannotCarryMoreItems))
		return
	default: // petitem.PickupNoop and any other unhandled failure
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}

	l.broadcastGroundPickup(ground, pet.ObjectID())
	l.groundItems.Remove(ground)
	l.world.Despawn(ground)

	if result.Herb != nil {
		l.consumePetHerb(live, pet, petInv, result.Herb)
	}
	l.applyPersistActions(ctx, result.Persist)
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
		return
	}
	live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessagePetPutOnS1, res.ItemID))
}

func (l *GameClientLink) broadcastGroundPickup(ground *grounditem.Item, pickerID int32) {
	if l.world == nil {
		return
	}
	broadcastFrame(func() wire.Frame {
		return serverpackets.FrameGetItem(ground, pickerID)
	}, func(send func(frameReceiver)) {
		l.world.ForEachKnown(ground, func(o world.Tracked) {
			if receiver, ok := o.(frameReceiver); ok {
				send(receiver)
			}
		})
	})
}
