package network

import (
	"context"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	enchantflow "github.com/fatal10110/acis_golang/internal/gameserver/enchant"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
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
// The lane is the row owner's at the moment the action is produced, so one
// owner's writes of one row keep their production order. The state itself is
// read when the write runs, not frozen here, because a lane is not enough on
// its own: an item that changes hands has its next write queued on the new
// owner's lane, and if the old owner's lane drains later, a frozen snapshot
// would overwrite the row with the previous owner. Reading at write time
// means every queued write lands the row's current state, so two lanes
// writing one row converge instead of fighting. item.Instance.Snapshot is
// mutex-guarded, so the read is safe from a worker goroutine.
//
// An item that has been destroyed or moved out of the world by the time the
// write runs becomes a delete, the same rule task.ItemInstances.addToBatch
// applies: an ItemStore save is an upsert, so writing that state as a row
// would resurrect what another writer has already deleted.
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
			insert := action.Action == invops.PersistSave
			inst := action.Item
			l.persist.Enqueue(inst.Snapshot().OwnerID, func() {
				ctx, cancel := context.WithTimeout(context.Background(), livePlayerDetachSaveTimeout)
				defer cancel()
				st := inst.Snapshot()
				if st.Count <= 0 || st.Location == item.LocationVoid {
					if err := l.items.Delete(ctx, st.ObjectID); err != nil {
						l.log.Error().Err(err).Int32("object_id", st.ObjectID).Msg("delete item")
					}
					return
				}
				write, op := l.items.UpdateState, "update item"
				if insert {
					write, op = l.items.SaveState, "save item"
				}
				if err := write(ctx, st); err != nil {
					l.log.Error().Err(err).Int32("object_id", st.ObjectID).Msg(op)
				}
			})
		case invops.PersistDelete:
			objectID := action.ObjectID
			l.persist.Enqueue(action.OwnerID, func() {
				ctx, cancel := context.WithTimeout(context.Background(), livePlayerDetachSaveTimeout)
				defer cancel()
				if err := l.items.Delete(ctx, objectID); err != nil {
					l.log.Error().Err(err).Int32("object_id", objectID).Msg("delete item")
				}
			})
		}
	}
}

func (l *GameClientLink) rollEnchant() float64 {
	if l.enchantRoll != nil {
		return l.enchantRoll()
	}
	return rnd.GetFloat(1)
}
