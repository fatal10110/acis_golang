package network

import (
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
	"github.com/fatal10110/acis_golang/internal/gameserver/petitem"
)

// partyLootView returns the party whose loot rule a ground item live (or
// its pet) picks up follows: live's own, unless the item is a cursed
// weapon, which always stays with whoever picks it up.
func (l *GameClientLink) partyLootView(live *livePlayer, ground *grounditem.Item) (party.View[*livePlayer], bool) {
	if l.parties == nil {
		return party.View[*livePlayer]{}, false
	}
	if l.cursedWeapons != nil {
		if _, cursed := l.cursedWeapons.Weapon(ground.ItemID()); cursed {
			return party.View[*livePlayer]{}, false
		}
	}
	return l.parties.View(live.ObjectID())
}

// pickupGroundForParty finishes live's pickup of the claimed ground item
// under its party's loot rule and reports whether the item was taken. Only
// finders-keepers needs room in the picker's own inventory: the other rules
// hand the item to a member with room, or to the picker when none has any.
// Adena is shared among the members in range and the picked-up stack
// destroyed; anything else goes to the member the rule picks, and the
// others are told who took it.
func (l *GameClientLink) pickupGroundForParty(live *livePlayer, ground *grounditem.Item, view party.View[*livePlayer]) bool {
	inv := live.Inventory()
	st := ground.Instance.Snapshot()
	if view.Loot == party.LootFindersKeepers && !inv.ValidateCapacity(inv.SlotsNeededFor(&ground.Instance, ground.Template)) {
		live.SendFrame(serverpackets.FrameActionFailed())
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSlotsFull))
		return false
	}
	if invops.LootLocked(st.OwnerID, live) {
		live.SendFrame(serverpackets.FrameActionFailed())
		live.SendFrame(failedPickupFrame(ground.ItemID(), ground.Count()))
		return false
	}
	if st.TemplateID == item.AdenaID {
		l.destroyGroundRow(st)
		l.takeGroundFromWorld(live, ground)
		l.shareAdena(view.Members, st.Count, live)
		l.lockPickupParalysis(live)
		return true
	}
	looter, members := l.groundLooter(live, st.TemplateID)
	if !l.storeGroundWith(looter.Inventory(), ground) {
		live.SendFrame(serverpackets.FrameActionFailed())
		return false
	}
	l.takeGroundFromWorld(live, ground)
	sendPartyLootNotice(members, looter, false, st.TemplateID, st.Count, st.EnchantLevel)
	looter.ItemAdded(event.ItemObtained{ItemID: st.TemplateID, Count: st.Count, EnchantLevel: st.EnchantLevel, Notice: event.ObtainPickup})
	l.lockPickupParalysis(live)
	return true
}

// petPickupForParty finishes a pet's pickup of the claimed ground item under
// its owner's party loot rule. Adena is shared among the members in range
// of the owner and the stack destroyed. Anything else goes into the pet's
// inventory when the rule picks the owner, who alone hears the pet pick it
// up, or else to the member it picks, and the others are told who took it.
func (l *GameClientLink) petPickupForParty(owner *livePlayer, pet *summon.Actor, petInv *itemcontainer.Inventory, ground *grounditem.Item, view party.View[*livePlayer]) {
	st := ground.Instance.Snapshot()
	if st.TemplateID == item.AdenaID {
		l.destroyGroundRow(st)
		l.takeGroundFromWorldByPet(owner, pet, ground)
		l.shareAdena(view.Members, st.Count, owner)
		return
	}
	looter, members := l.groundLooter(owner, st.TemplateID)
	if looter == owner {
		end := l.itemInstances.BeginOperationTaking([]int32{st.ObjectID}, petInv.OwnerID())
		defer end()
		persist, ok := petitem.StoreGround(petInv, ground)
		if !ok {
			ground.Release()
			owner.SendFrame(serverpackets.FrameActionFailed())
			return
		}
		l.applyPersistActions(persist)
		end()
		l.takeGroundFromWorldByPet(owner, pet, ground)
		owner.SendFrame(petPickedFrame(st))
		return
	}
	if !l.storeGroundWith(looter.Inventory(), ground) {
		ground.Release()
		owner.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	l.takeGroundFromWorldByPet(owner, pet, ground)
	sendPartyLootNotice(members, looter, false, st.TemplateID, st.Count, st.EnchantLevel)
	looter.ItemAdded(event.ItemObtained{ItemID: st.TemplateID, Count: st.Count, EnchantLevel: st.EnchantLevel, Notice: event.ObtainPickup})
}

// groundLooter picks the member that takes a ground item picker (or its
// pet) picked up: alive, with room, and within party range of picker. A
// picker that has left its party meanwhile keeps the item.
func (l *GameClientLink) groundLooter(picker *livePlayer, itemID int32) (*livePlayer, []*livePlayer) {
	looter, members, ok := l.parties.Looter(picker.ObjectID(), false, l.partyLootEligible(itemID, picker))
	if !ok || looter == nil || looter.Inventory() == nil {
		return picker, nil
	}
	return looter, members
}

// storeGroundWith moves the claimed ground item into inv, with no slot or
// loot-lock check, and persists the move.
func (l *GameClientLink) storeGroundWith(inv *itemcontainer.Inventory, ground *grounditem.Item) bool {
	if inv == nil {
		return false
	}
	end := l.itemInstances.BeginOperationTaking([]int32{ground.ObjectID()}, inv.OwnerID())
	defer end()
	res, failure := l.inventory.TakeGround(inv, &ground.Instance)
	l.applyPersistActions(res.Persist)
	return failure == invops.PickupOK
}

// destroyGroundRow deletes the stored row of a ground item that is shared
// out instead of carried.
func (l *GameClientLink) destroyGroundRow(st item.InstanceState) {
	end := l.itemInstances.BeginOperationTaking([]int32{st.ObjectID})
	defer end()
	l.applyPersistActions([]invops.Persist{invops.Delete(st.OwnerID, st.ObjectID)})
}

// takeGroundFromWorldByPet takes ground out of the world as pet's pickup:
// GetItem in the pet's name, DeleteObject, then the owner-named attention
// line.
func (l *GameClientLink) takeGroundFromWorldByPet(owner *livePlayer, pet *summon.Actor, ground *grounditem.Item) {
	l.broadcastGroundPickup(ground, pet.ObjectID())
	l.groundItems.Remove(ground)
	l.world.Despawn(ground)
	l.broadcastPetPickupAttention(owner, ground)
}
