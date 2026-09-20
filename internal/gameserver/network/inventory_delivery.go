package network

import (
	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
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

func (d *playerInventoryDelivery) UpdateInventoryWeight(inv *itemcontainer.Inventory) {
	if d == nil || d.live == nil || d.character == nil || d.live.detached() {
		return
	}
	d.live.SendFrame(serverpackets.FrameStatusUpdate(d.live.ObjectID(), []serverpackets.StatusAttribute{{
		Type: serverpackets.StatusCurrentLoad, Value: inv.TotalWeight(),
	}}))
	d.character.RefreshWeightPenalty()
}

type petInventoryDelivery struct {
	updates *task.InventoryUpdates
	live    *livePlayer
	state   *world.State
	log     zerolog.Logger
}

func (d *petInventoryDelivery) QueueInventoryUpdate(inv *itemcontainer.Inventory) {
	if d == nil || d.updates == nil || d.live == nil || d.state == nil || d.live.detached() {
		return
	}
	obj, ok := d.state.Summon(d.live.ObjectID())
	if !ok {
		return
	}
	pet, ok := obj.(*summon.Actor)
	if !ok || pet.PetInventory() != inv {
		return
	}
	d.updates.Add(inv, &petInventoryOwner{live: d.live, pet: pet, log: d.log})
}

// TODO(#2381): match PetInventory.updateWeight's pet status and info refresh.
func (*petInventoryDelivery) UpdateInventoryWeight(*itemcontainer.Inventory) {}

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
