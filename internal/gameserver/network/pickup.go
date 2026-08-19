package network

import (
	"context"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

const pickupAttentionRadius = 1400

// pickupParalyzeLock is how long a successful ground-item pickup briefly
// paralyzes the picking-up player, discouraging rapid repeated pickups.
const pickupParalyzeLock = 200 * time.Millisecond

// pickupLiveGroundItem handles an Action click on a ground item: it validates
// and moves the item into live's own inventory,
// then removes it from the visible world. It reports whether target was a
// ground item at all — false lets the caller fall through to its normal
// attack-target handling. Every other outcome answers with ActionFailed (in
// addition to any explanatory system message) so a rejected pickup releases
// the client's pending action instead of leaving it waiting for a response
// that never comes — the same failure shape as an unanswered attack click.
func (l *GameClientLink) pickupLiveGroundItem(ctx context.Context, live *livePlayer, target world.Tracked) bool {
	ground, ok := target.(*grounditem.Item)
	if !ok {
		return false
	}
	if live == nil {
		return true
	}
	if l.world == nil || l.groundItems == nil || ground.Template == nil || ground.Count() <= 0 {
		live.SendFrame(serverpackets.FrameActionFailed())
		return true
	}
	if blocked, deferrable := livePickupBlockedDeferrable(live); blocked {
		// pickupLiveGroundItem is only reached with a shift-clicked target
		// via a walk already in flight, and a shift click never starts one
		// (see walkOrForwardPickup) — so shift is always false here.
		l.deferOrFailPickup(ctx, live, ground, false, deferrable)
		return true
	}
	if !groundPickupInRange(live, ground) {
		live.SendFrame(serverpackets.FrameActionFailed())
		return true
	}
	if l.trades != nil && l.trades.HasActive(live.ObjectID()) {
		live.SendFrame(serverpackets.FrameActionFailed())
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotPickupOrUseItemTrading))
		return true
	}
	inv := live.Inventory()
	if inv == nil {
		live.SendFrame(serverpackets.FrameActionFailed())
		return true
	}

	// A herb is used the instant it is picked up and never reaches the
	// inventory: it carries no client icon there, so storing it would leave
	// an unusable blank slot. The capacity and loot-lock gates still run
	// first, in the reference's order — a herb needs no slot, so the capacity
	// check only rejects an inventory already past its limit.
	if ground.Herb() {
		groundState := ground.Instance.Snapshot()
		if !inv.ValidateCapacity(inv.SlotsNeededFor(&ground.Instance, ground.Template)) {
			live.SendFrame(serverpackets.FrameActionFailed())
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSlotsFull))
			return true
		}
		if invops.LootLocked(groundState.OwnerID, live.ObjectID()) {
			live.SendFrame(serverpackets.FrameActionFailed())
			live.SendFrame(failedPickupFrame(ground.ItemID(), ground.Count()))
			return true
		}
		live.SendFrame(serverpackets.FrameActionFailed())
		l.broadcastGroundPickup(ground, live.ObjectID())
		l.groundItems.Remove(ground)
		l.world.Despawn(ground)
		l.lockPickupParalysis(live)
		l.consumeHerb(live, ground.ItemID())
		return true
	}

	res, failure := l.inventory.PickupGround(inv, &ground.Instance, ground.Template, live.ObjectID())
	switch failure {
	case invops.PickupOK:
	case invops.PickupSlotsFull:
		live.SendFrame(serverpackets.FrameActionFailed())
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSlotsFull))
		return true
	case invops.PickupLootLocked:
		live.SendFrame(serverpackets.FrameActionFailed())
		live.SendFrame(failedPickupFrame(ground.ItemID(), ground.Count()))
		return true
	default: // invops.PickupNoop and any other unhandled failure
		live.SendFrame(serverpackets.FrameActionFailed())
		return true
	}

	// A successful pickup still has to release the pending action the client
	// registered when it accepted the click. GetItem, DeleteObject and
	// InventoryUpdate all describe the world, not the action's outcome, so
	// without this the client keeps its input locked and stops responding to
	// every later click — the same failure shape as an unanswered rejection.
	live.SendFrame(serverpackets.FrameActionFailed())

	l.broadcastGroundPickup(ground, live.ObjectID())
	l.broadcastPickupAttention(live, ground)
	l.groundItems.Remove(ground)
	l.world.Despawn(ground)
	l.lockPickupParalysis(live)

	l.applyPersistActions(ctx, res.Persist)
	return true
}

// lockPickupParalysis briefly paralyzes live after a successful pickup,
// clearing the lock once pickupParalyzeLock elapses. Entering and exiting
// the lock are each a single atomic step (see enterPickupLock/
// exitPickupLock) so a click racing the unlock can never observe paralysis
// lifted with the lock still nominally held, or vice versa.
func (l *GameClientLink) lockPickupParalysis(live *livePlayer) {
	gen := live.enterPickupLock()
	l.scheduleAfter(pickupParalyzeLock, func() {
		if !live.exitPickupLock(gen) {
			return
		}
		l.finishDeferredPickup(live)
	})
}

// livePickupBlockedDeferrable atomically reports whether live is currently blocked
// from picking up and, if so, whether that block is one finishDeferredPickup
// will later lift. Both reads happen inside the single pickupMu-held section
// enterPickupLock/exitPickupLock also use to flip pickupLocked and
// Paralyzed together — so a concurrent exitPickupLock can never be observed
// as still blocking by one read and already cleared by the other, which is
// what let a click land in the unlock instant, read blocked, then read
// not-deferrable, and get discarded instead of re-deferred (#1159).
//
// Crowd control is deliberately absent from the block set: the reference's
// pickup path gates nothing but flying (ItemInstance.java:145-184,
// PlayerAI.java:327-414), so a stunned, sleeping, paralyzed, or afraid
// player picks items up immediately. Only death (via liveItemOpsAllowed's
// dead-only check), the transient pickup lock (pickupLocked, the
// PlayerAI.java:412-413 200ms anti-mash gate), a non-standing pose,
// attacking, and casting defer — those are the busy states the reference's
// AI retries on its next think.
func livePickupBlockedDeferrable(live *livePlayer) (blocked, deferrable bool) {
	live.pickupMu.Lock()
	defer live.pickupMu.Unlock()
	attacking := live.attack != nil && live.attack.AttackingNow()
	blocked = !liveItemOpsAllowed(live) || live.pickupLocked || !live.Standing() || attacking || (live.cast != nil && live.cast.CastingNow())
	deferrable = attacking || live.pickupLocked
	return
}

func groundPickupInRange(live *livePlayer, ground *grounditem.Item) bool {
	sx, sy, sz := live.Position()
	gx, gy, gz := ground.Position()
	return location.In3DRange(sx, sy, sz, gx, gy, gz, groundPickupInteractionDistance)
}

func (l *GameClientLink) broadcastPickupAttention(live *livePlayer, ground *grounditem.Item) {
	if l.world == nil || live == nil || ground == nil || ground.Template == nil {
		return
	}
	switch ground.Template.Kind {
	case item.KindArmor, item.KindWeapon:
	default:
		return
	}

	st := ground.Instance.Snapshot()
	frame := func() wire.Frame {
		if st.EnchantLevel > 0 {
			return serverpackets.FrameSystemMessageStringNumberItemName(
				serverpackets.SystemMessageAttentionS1PickedUpS2S3,
				live.Name,
				int32(st.EnchantLevel),
				st.TemplateID,
			)
		}
		return serverpackets.FrameSystemMessageStringItemName(
			serverpackets.SystemMessageAttentionS1PickedUpS2,
			live.Name,
			st.TemplateID,
		)
	}
	broadcastFrame(frame, func(send func(frameReceiver)) {
		send(live)
		l.world.ForEachKnownInRadius(live, pickupAttentionRadius, func(o world.Tracked) {
			if receiver, ok := o.(frameReceiver); ok {
				send(receiver)
			}
		})
	})
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
