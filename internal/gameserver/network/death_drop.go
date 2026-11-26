package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// deathDropOffset is how far from the dead player, along each axis, an item
// its death drops may land.
const deathDropOffset = 25

// dropItemsOnDeath drops the items live's death costs it at e's rates
// (inventory.DeathDrops). A drop names the item to live and lands it on
// the ground near live, as an item live dropped. Nothing else is said: the
// inventory changes reach live as the usual batched InventoryUpdate, and
// observers see the gear change on live's next appearance refresh.
func (l *GameClientLink) dropItemsOnDeath(live *livePlayer, e event.DeathItemDrop) {
	inv := live.Inventory()
	if l.inventory == nil || l.belongings.groundItems == nil {
		return
	}
	unequipped := func(changed []*item.Instance) {
		l.applyEquipItemStats(live, inv, invops.Result{EquipmentChanged: true, Changed: changed})
	}
	for inst := range invops.DeathDrops(inv, live.accessLevel().IsGM, invops.DeathDropRates(e), live, unequipped) {
		l.dropDeathItem(live, inv, inst)
	}
}

// dropDeathItem drops all of inst from live's inventory onto the ground at
// a random spot within deathDropOffset of live that geodata lets the item
// reach, and tells live which item it dropped. An item that is no longer
// there is answered as a drop of too few items.
func (l *GameClientLink) dropDeathItem(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance) {
	l.dropHeldItem(live, inv, inst, false)
}

// dropHeldItem is dropDeathItem returning the ground item it laid down.
// cursed drops a cursed weapon: the item's droppable flag cannot refuse
// it, the ground cleanup leaves it be, and it lies claimed until the
// caller recorded it (cursedweapon.Manager.PlaceOnGround), so no pickup
// takes it before. ok is false when nothing dropped.
func (l *GameClientLink) dropHeldItem(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance, cursed bool) (*grounditem.Item, bool) {
	var (
		res invops.DropResult
		ok  bool
		err error
	)
	if cursed {
		res, ok, err = l.inventory.ForceDropItem(inv, inst.ObjectID)
	} else {
		res, ok, err = l.inventory.DropItem(inv, inst.ObjectID, inst.Snapshot().Count)
	}
	if err != nil {
		l.log.Error().Err(err).Int32("object_id", inst.ObjectID).Msg("drop item on death")
	}
	if !ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughItems))
		return nil, false
	}
	ground, err := grounditem.New(*res.Dropped, res.Template)
	if err != nil {
		l.log.Error().Err(err).Int32("object_id", inst.ObjectID).Msg("build item dropped on death")
		return nil, false
	}
	ground.SetDestroyProtected(cursed)
	if cursed {
		ground.Claim()
	}
	live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageYouDroppedS1, res.Template.ID))
	x, y, z := live.Position()
	tx := x + rnd.GetRange(-deathDropOffset, deathDropOffset)
	ty := y + rnd.GetRange(-deathDropOffset, deathDropOffset)
	tz := z
	if l.geo != nil {
		at := l.geo.ValidLocation(x, y, z, tx, ty, tz)
		tx, ty, tz = at.X, at.Y, at.Z
	}
	l.belongings.groundItems.Drop(ground, task.DropOptions{
		X:             tx,
		Y:             ty,
		Z:             tz,
		Heading:       live.CurrentHeading(),
		PlayerDropped: true,
		DropperID:     live.ObjectID(),
	})
	return ground, true
}
