package network

import (
	"context"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// pickupLiveGroundItem handles a second Action click on an already-selected
// ground item: it validates and moves the item into live's own inventory,
// then removes it from the visible world. It reports whether target was a
// ground item at all — false lets the caller fall through to its normal
// attack-target handling.
func (l *GameClientLink) pickupLiveGroundItem(ctx context.Context, live *livePlayer, target world.Tracked) bool {
	ground, ok := target.(*grounditem.Item)
	if !ok {
		return false
	}
	if live == nil || l.world == nil || l.groundItems == nil || ground.Template == nil || ground.Count() <= 0 {
		return true
	}
	if !liveItemOpsAllowed(live) || !live.Standing() {
		live.SendFrame(serverpackets.FrameActionFailed())
		return true
	}
	if !groundPickupInRange(live, ground) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetTooFar))
		return true
	}
	if l.trades != nil && l.trades.HasActive(live.ObjectID()) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotPickupOrUseItemTrading))
		return true
	}
	inv := live.Inventory()
	if inv == nil {
		return true
	}

	res, failure := l.inventoryService().PickupGround(inv, &ground.Instance, ground.Template, live.ObjectID())
	switch failure {
	case invops.PickupOK:
	case invops.PickupNoop:
		return true
	case invops.PickupSlotsFull:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSlotsFull))
		return true
	case invops.PickupLootLocked:
		live.SendFrame(failedPickupFrame(ground.ItemID(), ground.Count()))
		return true
	default:
		return true
	}

	l.broadcastGroundPickup(ground, live.ObjectID())
	l.groundItems.Remove(ground)
	l.world.Despawn(ground)

	l.applyPersistActions(ctx, res.Persist)
	l.sendInventoryUpdate(live, inv)
	return true
}

func groundPickupInRange(live *livePlayer, ground *grounditem.Item) bool {
	sx, sy, sz := live.Position()
	gx, gy, gz := ground.Position()
	return location.In3DRange(sx, sy, sz, gx, gy, gz, groundPickupInteractionDistance)
}

// failedPickupFrame mirrors the reference server's loot-locked messaging:
// adena reports only the amount, a single non-adena item names itself, and
// a stack of more than one names itself alongside its count.
func failedPickupFrame(templateID int32, count int) wire.Frame {
	switch {
	case templateID == item.AdenaID:
		return serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageFailedToPickupAdena, int32(count))
	case count > 1:
		return serverpackets.FrameSystemMessageItemNameItemNumber(serverpackets.SystemMessageFailedToPickupS2S1S, templateID, int32(count))
	default:
		return serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageFailedToPickupS1, templateID)
	}
}
