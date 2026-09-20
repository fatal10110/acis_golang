package network

import (
	"context"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	enchantflow "github.com/fatal10110/acis_golang/internal/gameserver/enchant"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
)

func (l *GameClientLink) enchantStateStore() *enchantflow.State {
	if l.enchantState == nil {
		l.enchantState = enchantflow.NewState()
	}
	return l.enchantState
}

func (l *GameClientLink) enchantService() *enchantflow.Service {
	if l.enchant == nil {
		l.enchant = enchantflow.NewService(l.enchantStateStore(), l.ids, l.rollEnchant)
	}
	return l.enchant
}

func (l *GameClientLink) useEnchantScroll(live *livePlayer, scroll *item.Instance) bool {
	if live == nil || scroll == nil {
		return false
	}
	result, ok := l.enchantService().UseScroll(live.ObjectID(), scroll)
	if !ok {
		return false
	}
	if result.FirstSelect {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSelectItemToEnchant))
	}
	live.SendFrame(serverpackets.FrameChooseInventoryItem(result.ScrollItemID))
	return true
}

func (l *GameClientLink) enchantLiveItem(ctx context.Context, live *livePlayer, req clientpackets.RequestEnchantItem) {
	if !liveItemOpsAllowed(live) || req.ObjectID == 0 {
		return
	}
	inv := live.Inventory()
	if inv == nil {
		return
	}

	result, err := l.enchantService().EnchantItem(live.ObjectID(), inv, req.ObjectID)
	if err != nil {
		l.log.Error().Err(err).Msg("enchant item")
	}
	l.applyPersistActions(result.Persist)
	if len(result.Steps) == 0 {
		return
	}
	l.applyEnchantSteps(live, result.Steps)
}

func (l *GameClientLink) cancelActiveEnchant(live *livePlayer) {
	if live == nil {
		return
	}
	result := l.enchantService().Cancel(live.ObjectID())
	l.applyEnchantSteps(live, result.Steps)
}

func (l *GameClientLink) applyEnchantSteps(live *livePlayer, steps []enchantflow.Step) {
	for _, step := range steps {
		switch step.Kind {
		case enchantflow.StepSystemMessage:
			l.sendEnchantMessage(live, step.Message)
		case enchantflow.StepEnchantResult:
			live.SendFrame(serverpackets.FrameEnchantResult(enchantResult(step.EnchantResult)))
		case enchantflow.StepBroadcastEquipment:
			l.broadcastEquipmentChange(live)
		}
	}
}

func (l *GameClientLink) sendEnchantMessage(live *livePlayer, message enchantflow.Message) {
	switch message.Code {
	case enchantflow.MessageSelectItemToEnchant:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSelectItemToEnchant))
	case enchantflow.MessageEnchantScrollCancelled:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageEnchantScrollCancelled))
	case enchantflow.MessageInappropriateEnchantCondition:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInappropriateEnchantCondition))
	case enchantflow.MessageNotEnoughItems:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughItems))
	case enchantflow.MessageS1SuccessfullyEnchanted:
		live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageS1SuccessfullyEnchanted, message.ItemID))
	case enchantflow.MessageS1S2SuccessfullyEnchanted:
		live.SendFrame(serverpackets.FrameSystemMessageNumberItemName(serverpackets.SystemMessageS1S2SuccessfullyEnchanted, message.Number, message.ItemID))
	case enchantflow.MessageBlessedEnchantFailed:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageBlessedEnchantFailed))
	case enchantflow.MessageEarnedS2S1S:
		live.SendFrame(serverpackets.FrameSystemMessageItemNameItemNumber(serverpackets.SystemMessageEarnedS2S1S, message.ItemID, message.Number))
	case enchantflow.MessageEnchantmentFailedS1S2Evaporated:
		live.SendFrame(serverpackets.FrameSystemMessageNumberItemName(serverpackets.SystemMessageEnchantmentFailedS1S2Evaporated, message.Number, message.ItemID))
	case enchantflow.MessageEnchantmentFailedS1Evaporated:
		live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageEnchantmentFailedS1Evaporated, message.ItemID))
	}
}

func enchantResult(result enchantflow.ResultCode) serverpackets.EnchantResult {
	switch result {
	case enchantflow.ResultSuccess:
		return serverpackets.EnchantResultSuccess
	case enchantflow.ResultUnsuccess:
		return serverpackets.EnchantResultUnsuccess
	case enchantflow.ResultBrokenNoCrystals:
		return serverpackets.EnchantResultBrokenNoCrystals
	case enchantflow.ResultBrokenWithCrystals:
		return serverpackets.EnchantResultBrokenWithCrystals
	default:
		return serverpackets.EnchantResultCancelled
	}
}

// applyPersistActions queues actions' item-row writes on the persistence
// worker instead of writing them here: this runs on an actor queue, where a
// slow database would hold a pool worker and stall unrelated actors.
//
// Each write carries the state this queue produced it with, and takes its
// place in that row's write order before it is queued (persist.Order). The
// lane it runs on is the row owner's at that moment, so one owner's writes of
// one row also keep their order; the reservation is what covers the rest,
// because a row does not stay with one owner. An item that changes hands has
// its next write queued on the new owner's lane, a destroyed item's delete
// comes from the persistence tick's own lane, and a dropped item is picked up
// as a new instance carrying the same object id — in each case a second lane
// writes the row this one is about to, and only the reservation decides which
// of them the row keeps.
//
// Both a new row and a changed one are written as the same upsert, which is
// what the persistence tick's own batch does for every live item
// (task.ItemInstances.addToBatch). The distinction cannot survive ordering:
// the write that creates a row may be the one a later write supersedes, and a
// plain UPDATE from that later write would then touch nothing and lose the
// row altogether.
//
// A count-zero pet collar is the one row whose pets-row delete needs the
// collar's own lane (task.ItemInstances.laneKey); the item-row delete queued
// here touches no pets row, and both deletes are idempotent, so it stays on
// the owner's lane with everything else.
func (l *GameClientLink) applyPersistActions(actions []invops.Persist) {
	if l.items == nil {
		return
	}
	for _, action := range actions {
		switch action.Action {
		case invops.PersistSave, invops.PersistUpdate:
			if action.Item == nil {
				continue
			}
			st := action.Item.Snapshot()
			l.queueItemWrite(st.OwnerID, l.itemWrites.Reserve(st.ObjectID), func() {
				ctx, cancel := context.WithTimeout(context.Background(), livePlayerDetachSaveTimeout)
				defer cancel()
				if err := l.items.SaveState(ctx, st); err != nil {
					l.log.Error().Err(err).Int32("object_id", st.ObjectID).Msg("save item")
				}
			})
		case invops.PersistDelete:
			objectID := action.ObjectID
			l.queueItemWrite(action.OwnerID, l.itemWrites.Reserve(objectID), func() {
				ctx, cancel := context.WithTimeout(context.Background(), livePlayerDetachSaveTimeout)
				defer cancel()
				if err := l.items.Delete(ctx, objectID); err != nil {
					l.log.Error().Err(err).Int32("object_id", objectID).Msg("delete item")
				}
			})
		}
	}
}

// itemWriteRowWait is how long a queued item write waits for its row before
// giving its lane back. It only has to be long enough that an uncontended row
// is taken on the first try; the wait for a row another write is holding is
// paid by re-queueing, not by sitting on the lane.
const itemWriteRowWait = 20 * time.Millisecond

// queueItemWrite runs write on ownerID's persistence lane once reserved's row
// is free. A lane serves every owner that maps to it (persist.Lanes of them
// for the whole world), so a write that simply waited for its row would stall
// the character, shortcut, skill and pet writes queued behind it — and the
// row can be held by the persistence tick's chunk for its whole transaction.
// This comes back to the lane instead, which keeps the lane draining and
// still lands the write in its reserved place.
//
// Coming back puts the write behind whatever the lane has taken meanwhile,
// including a flush marker pushed after it was first queued, so the write is
// also booked as work the lane owes: awaitPersistence waits for a lane to
// report that what it queued has run, and a restart reads the items table
// straight after. The debt is settled however the write ends — landed,
// dropped as superseded, or cancelled.
func (l *GameClientLink) queueItemWrite(ownerID int32, reserved *persist.Write, write func()) {
	owed := l.persist.Owe(ownerID)
	var attempt func()
	attempt = func() {
		if reserved.TryRun(itemWriteRowWait, func([]int32) { write() }) {
			owed.Settle()
			return
		}
		if !l.persist.Enqueue(ownerID, attempt) {
			// Shutdown: nothing will come back for this, so take the wait
			// here rather than dropping a write the row is still owed.
			reserved.Run(func([]int32) { write() })
			owed.Settle()
		}
	}
	if !l.persist.Enqueue(ownerID, attempt) {
		reserved.Cancel()
		owed.Settle()
	}
}

func (l *GameClientLink) rollEnchant() float64 {
	if l.enchantRoll != nil {
		return l.enchantRoll()
	}
	return rnd.GetFloat(1)
}
