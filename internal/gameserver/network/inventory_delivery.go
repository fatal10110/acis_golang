package network

import (
	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

type playerInventoryDelivery struct {
	updates   *task.InventoryUpdates
	live      *livePlayer
	character *player.Character
}

func (d *playerInventoryDelivery) QueueInventoryUpdate(inv *itemcontainer.Inventory) {
	if d == nil || d.updates == nil || d.live == nil || d.live.detached() {
		return
	}
	d.updates.Add(inv, d.live)
}

// InventoryItemsRemoved takes the item shortcuts of every instance that
// left the inventory off the bar. The removal can run on any goroutine, so
// the deletion goes to the player's queue, which owns the bar; an item that
// leaves before a detached player's next login is dropped from the bar when
// the shortcuts are restored.
func (d *playerInventoryDelivery) InventoryItemsRemoved(_ *itemcontainer.Inventory, objectIDs []int32) {
	if d == nil || d.live == nil || d.live.detached() {
		return
	}
	live := d.live
	postLive(live, func() {
		for _, objectID := range objectIDs {
			live.link.deleteTargetShortcuts(live, shortcut.Item, objectID)
		}
	})
}

func (d *playerInventoryDelivery) UpdateInventoryWeight(inv *itemcontainer.Inventory) {
	if d == nil || d.live == nil || d.character == nil || d.live.detached() {
		return
	}
	d.live.SendFrame(serverpackets.FrameStatusUpdate(d.live.ObjectID(), []serverpackets.StatusAttribute{{
		Type: serverpackets.StatusCurrentLoad, Value: inv.TotalWeight(),
	}}))
	d.character.RefreshWeightPenalty()
}

// petInventoryDelivery reports a pet inventory's changes to the session the
// pet answers to, which changes when a pet corpse left behind is handed to
// its owner's next session.
type petInventoryDelivery struct {
	updates *task.InventoryUpdates
	ownerID int32
	state   *world.State
	log     zerolog.Logger
}

func (d *petInventoryDelivery) QueueInventoryUpdate(inv *itemcontainer.Inventory) {
	if d == nil || d.updates == nil {
		return
	}
	if pet, live, ok := d.pet(inv); ok {
		d.updates.Add(inv, &petInventoryOwner{live: live, pet: pet, log: d.log})
	}
}

// UpdateInventoryWeight republishes the pet's status (owner PetStatusUpdate,
// NpcInfo to other watchers), after its weight-penalty band refresh, and
// then the owner's PetInfo window, which carries the new carried weight.
func (d *petInventoryDelivery) UpdateInventoryWeight(inv *itemcontainer.Inventory) {
	if pet, _, ok := d.pet(inv); ok {
		pet.UpdateStatus()
		sendSummonInfosToOwner(pet)
	}
}

// pet returns the owner's pet while inv is still its inventory, with the
// connected session it answers to.
func (d *petInventoryDelivery) pet(inv *itemcontainer.Inventory) (*summon.Actor, *livePlayer, bool) {
	if d == nil || d.state == nil {
		return nil, nil, false
	}
	obj, ok := d.state.Summon(d.ownerID)
	if !ok {
		return nil, nil, false
	}
	pet, ok := obj.(*summon.Actor)
	if !ok || pet.PetInventory() != inv {
		return nil, nil, false
	}
	live, ok := liveSummonOwner(pet)
	if !ok || live.detached() {
		return nil, nil, false
	}
	return pet, live, true
}

// ownerItemPersister is the live persistence dependency of one inventory: its
// items' writes register with the lazy item task under the inventory's owner,
// which names the row's lane even for a destroy that zeroes the item's own.
// Logout and unsummon end it with Container.ReleasePersistence.
type ownerItemPersister struct {
	instances *task.ItemInstances
	ownerID   int32
}

func (p *ownerItemPersister) Persist(inst *item.Instance) { p.instances.AddOwned(p.ownerID, inst) }

// itemPersister returns the persistence dependency for ownerID's inventory,
// or nil when no item persistence task is wired.
func (l *GameClientLink) itemPersister(ownerID int32) item.Persister {
	if l.itemInstances == nil {
		return nil
	}
	return &ownerItemPersister{instances: l.itemInstances, ownerID: ownerID}
}
