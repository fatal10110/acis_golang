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

// dropItemsOnDeath rolls the items live's death costs it at e's rates. A
// game master keeps everything unless the server lets it drop. Otherwise
// the death drops nothing unless its first roll passes; then every item
// the death may drop is taken in container order: a worn item comes off
// whether or not it then drops, and each rolls its own chance until the
// limit has dropped. A drop names the item to live and lands it on the
// ground near live, as an item live dropped. Nothing else is said: the
// inventory changes reach live as the usual batched InventoryUpdate, and
// observers see the gear change on live's next appearance refresh.
func (l *GameClientLink) dropItemsOnDeath(live *livePlayer, e event.DeathItemDrop) {
	rules := l.playerConfig.DeathDrop
	if live.accessLevel().IsGM && !rules.GMDrops {
		return
	}
	inv := live.Inventory()
	if inv == nil || l.inventory == nil || l.groundItems == nil {
		return
	}
	if e.Chance <= 0 || live.Roll(100) >= e.Chance {
		return
	}
	dropped := 0
	for _, inst := range inv.Items() {
		tmpl, ok := invops.DeathDroppable(inv, inst, rules.Kept)
		if !ok || live.ControlItemInUse(inst.ObjectID) {
			continue
		}
		chance := e.ItemChance
		if inst.Equipped() {
			chance = e.EquipChance
			if tmpl.Weapon != nil {
				chance = e.WeaponChance
			}
			if changed := inv.UnequipItem(inst); len(changed) > 0 {
				l.applyEquipItemStats(live, inv, invops.Result{EquipmentChanged: true, Changed: changed})
			}
		}
		if live.Roll(100) >= chance {
			continue
		}
		l.dropDeathItem(live, inv, inst)
		dropped++
		if dropped >= e.Limit {
			break
		}
	}
}

// dropDeathItem drops all of inst from live's inventory onto the ground at
// a random spot within deathDropOffset of live that geodata lets the item
// reach, and tells live which item it dropped. An item that is no longer
// there is answered as a drop of too few items.
func (l *GameClientLink) dropDeathItem(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance) {
	res, ok, err := l.inventory.DropItem(inv, inst.ObjectID, inst.Snapshot().Count)
	if err != nil {
		l.log.Error().Err(err).Int32("object_id", inst.ObjectID).Msg("drop item on death")
	}
	if !ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughItems))
		return
	}
	ground, err := grounditem.New(*res.Dropped, res.Template)
	if err != nil {
		l.log.Error().Err(err).Int32("object_id", inst.ObjectID).Msg("build item dropped on death")
		return
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
	l.groundItems.Drop(ground, task.DropOptions{
		X:             tx,
		Y:             ty,
		Z:             tz,
		Heading:       live.CurrentHeading(),
		PlayerDropped: true,
		DropperID:     live.ObjectID(),
	})
}
