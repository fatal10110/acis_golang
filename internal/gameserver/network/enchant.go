package network

import (
	"context"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	enchantflow "github.com/fatal10110/acis_golang/internal/gameserver/enchant"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
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
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	inv := live.Inventory()
	if inv == nil {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}

	result, err := l.enchantService().EnchantItem(live.ObjectID(), inv, req.ObjectID)
	if err != nil {
		l.log.Error().Err(err).Msg("enchant item")
	}
	l.applyPersistActions(ctx, result.Persist)
	if len(result.Steps) == 0 {
		// A target with no active scroll selected (or any other condition
		// EnchantItem treats as "nothing to do") produces no steps to
		// report. The request was still accepted and must still resolve,
		// or the client's pending enchant action never clears.
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	l.applyEnchantSteps(live, inv, result.Steps)
}

func (l *GameClientLink) cancelActiveEnchant(live *livePlayer) {
	if live == nil {
		return
	}
	result := l.enchantService().Cancel(live.ObjectID())
	l.applyEnchantSteps(live, live.Inventory(), result.Steps)
}

func (l *GameClientLink) applyEnchantSteps(live *livePlayer, inv *itemcontainer.Inventory, steps []enchantflow.Step) {
	for _, step := range steps {
		switch step.Kind {
		case enchantflow.StepSystemMessage:
			l.sendEnchantMessage(live, step.Message)
		case enchantflow.StepEnchantResult:
			live.SendFrame(serverpackets.FrameEnchantResult(enchantResult(step.EnchantResult)))
		case enchantflow.StepInventoryUpdate:
			l.sendInventoryUpdate(live, inv)
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

func (l *GameClientLink) applyPersistActions(ctx context.Context, actions []invops.Persist) {
	if l.items == nil {
		return
	}
	for _, action := range actions {
		switch action.Action {
		case invops.PersistSave:
			if action.Item == nil {
				continue
			}
			if err := l.items.Save(ctx, action.Item); err != nil {
				l.log.Error().Err(err).Int32("object_id", action.Item.ObjectID).Msg("save item")
			}
		case invops.PersistUpdate:
			if action.Item == nil {
				continue
			}
			if err := l.items.Update(ctx, action.Item); err != nil {
				l.log.Error().Err(err).Int32("object_id", action.Item.ObjectID).Msg("update item")
			}
		case invops.PersistDelete:
			if err := l.items.Delete(ctx, action.ObjectID); err != nil {
				l.log.Error().Err(err).Int32("object_id", action.ObjectID).Msg("delete item")
			}
		}
	}
}

func (l *GameClientLink) rollEnchant() float64 {
	if l.enchantRoll != nil {
		return l.enchantRoll()
	}
	return rnd.GetFloat(1)
}
