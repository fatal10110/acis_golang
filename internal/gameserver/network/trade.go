package network

import (
	"context"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	tradebook "github.com/fatal10110/acis_golang/internal/gameserver/trade"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

const tradeInteractionDistance = 150

func (l *GameClientLink) tradeBook() *tradebook.Book {
	if l.trades == nil {
		l.trades = tradebook.NewBook(time.Now)
	}
	return l.trades
}

func (l *GameClientLink) handleTradeRequest(live *livePlayer, req clientpackets.TradeRequest) {
	if live == nil || l.world == nil {
		return
	}
	target, ok := l.livePlayerByID(req.ObjectID)
	if !ok {
		return
	}
	if target.ObjectID() == live.ObjectID() || !world.Knows(live, target) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetIncorrect))
		return
	}

	switch l.tradeBook().Request(live.ObjectID(), target.ObjectID()).Status {
	case tradebook.RequestRequesterBusy:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageAlreadyTrading))
		return
	case tradebook.RequestTargetBusy:
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1IsBusyTryLater, target.Name))
		return
	}

	target.SendFrame(serverpackets.FrameSendTradeRequest(live.ObjectID()))
	live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageRequestS1ForTrade, target.Name))
}

func (l *GameClientLink) handleAnswerTradeRequest(live *livePlayer, req clientpackets.AnswerTradeRequest) {
	if live == nil {
		return
	}

	result := l.tradeBook().Answer(live.ObjectID(), req.Response == 1)
	if result.Status == tradebook.AnswerMissing {
		live.SendFrame(serverpackets.FrameSendTradeDone(false))
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetNotFound))
		return
	}

	requester, requesterOnline := l.livePlayerByID(result.RequesterID)
	if !requesterOnline {
		if result.Status == tradebook.AnswerAccepted {
			l.tradeBook().Cancel(live.ObjectID())
		}
		live.SendFrame(serverpackets.FrameSendTradeDone(false))
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetNotFound))
		return
	}
	if result.Status == tradebook.AnswerDenied {
		requester.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1DeniedTradeRequest, live.Name))
		return
	}
	if !l.validTradeParticipants(requester, live) {
		l.tradeBook().Cancel(live.ObjectID())
		live.SendFrame(serverpackets.FrameSendTradeDone(false))
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetNotFound))
		return
	}

	if !l.sendTradeStart(requester, live) || !l.sendTradeStart(live, requester) {
		l.cancelTradeByID(requester.ObjectID())
	}
}

func (l *GameClientLink) handleAddTradeItem(live *livePlayer, req clientpackets.AddTradeItem) {
	if live == nil || req.Count <= 0 {
		return
	}
	session, ok := l.tradeBook().Session(live.ObjectID())
	if !ok {
		return
	}
	partnerID, ok := session.PartnerID(live.ObjectID())
	if !ok {
		return
	}
	partner, ok := l.livePlayerByID(partnerID)
	if !ok || !l.validTradeParticipants(live, partner) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetNotFound))
		l.cancelTradeByID(live.ObjectID())
		return
	}

	result := l.tradeBook().AddItem(live.ObjectID(), live.Inventory(), req.ObjectID, int(req.Count))
	switch result.Status {
	case tradebook.AddNoSession:
		return
	case tradebook.AddSelfConfirmed:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageOnceTradeConfirmedCannotMove))
		return
	case tradebook.AddPartnerConfirmed:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotAdjustItemsAfterConfirm))
		return
	case tradebook.AddInvalidItem:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNothingHappened))
		return
	}

	snapshot := tradeItemSnapshot(result.Item)
	if frame, err := serverpackets.FrameTradeOwnAdd(snapshot, result.AddedCount, live.Inventory().Templates()); err == nil {
		live.SendFrame(frame)
	} else {
		l.log.Error().Err(err).Msg("build TradeOwnAdd")
		return
	}
	if frame, err := serverpackets.FrameTradeUpdate(snapshot, result.AvailableCount, live.Inventory().Templates()); err == nil {
		live.SendFrame(frame)
	} else {
		l.log.Error().Err(err).Msg("build TradeUpdate")
		return
	}
	if frame, err := serverpackets.FrameTradeItemUpdate(tradeItemUpdateEntries(result.Entries), live.Inventory().Templates()); err == nil {
		live.SendFrame(frame)
	} else {
		l.log.Error().Err(err).Msg("build TradeItemUpdate")
		return
	}
	if frame, err := serverpackets.FrameTradeOtherAdd(snapshot, result.AddedCount, partner.Inventory().Templates()); err == nil {
		partner.SendFrame(frame)
	} else {
		l.log.Error().Err(err).Msg("build TradeOtherAdd")
	}
}

func (l *GameClientLink) handleTradeDone(ctx context.Context, live *livePlayer, req clientpackets.TradeDone) {
	if live == nil {
		return
	}
	if req.Response != 1 {
		l.cancelTradeByID(live.ObjectID())
		return
	}

	session, ok := l.tradeBook().Session(live.ObjectID())
	if !ok {
		return
	}
	partnerID, ok := session.PartnerID(live.ObjectID())
	if !ok {
		return
	}
	partner, ok := l.livePlayerByID(partnerID)
	if !ok || !l.validTradeParticipants(live, partner) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetNotFound))
		return
	}

	result := l.tradeBook().Confirm(live.ObjectID())
	switch result.Status {
	case tradebook.DoneNoSession, tradebook.DoneAlreadyConfirmed:
		return
	case tradebook.DoneConfirmed:
		l.sendTradeConfirmed(live, partner)
		return
	}

	first, second, ok := l.tradeParticipants(result.Session)
	if !ok || !l.validTradeParticipants(first, second) {
		l.finishTrade(first, second, false)
		return
	}

	settlement := tradebook.Settle(result.Session, first.Inventory(), second.Inventory(), func(source, receiver *itemcontainer.Inventory, objectID int32, count int) bool {
		return l.transferTradeInventoryItem(ctx, source, receiver, objectID, count)
	})
	failMessage := tradeSettlementMessage(settlement.Status)
	if failMessage != 0 {
		first.SendFrame(serverpackets.FrameSystemMessage(failMessage))
		second.SendFrame(serverpackets.FrameSystemMessage(failMessage))
	}
	success := settlement.Status == tradebook.SettlementOK
	if success {
		l.sendInventoryUpdate(first, first.Inventory())
		l.sendInventoryUpdate(second, second.Inventory())
	}
	l.finishTrade(first, second, success)
}

func (l *GameClientLink) cancelActiveTrade(live *livePlayer) {
	if live == nil || l.trades == nil {
		return
	}
	l.cancelTradeByID(live.ObjectID())
}

func (l *GameClientLink) cancelTradeByID(playerID int32) {
	result := l.tradeBook().Cancel(playerID)
	if result.Status != tradebook.CancelDone {
		return
	}
	first, second, ok := l.tradeParticipants(result.Session)
	if !ok {
		return
	}
	first.SendFrame(serverpackets.FrameSendTradeDone(false))
	first.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1CanceledTrade, second.Name))
	second.SendFrame(serverpackets.FrameSendTradeDone(false))
	second.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1CanceledTrade, first.Name))
}

func (l *GameClientLink) finishTrade(first, second *livePlayer, success bool) {
	for _, live := range []*livePlayer{first, second} {
		if live == nil {
			continue
		}
		live.SendFrame(serverpackets.FrameSendTradeDone(success))
		if success {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTradeSuccessful))
		} else {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageExchangeHasEnded))
		}
	}
}

func (l *GameClientLink) sendTradeStart(live, partner *livePlayer) bool {
	live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageBeginTradeWithS1, partner.Name))
	frame, err := serverpackets.FrameTradeStart(partner.ObjectID(), live.Inventory().Items(), live.Inventory().Templates())
	if err != nil {
		l.log.Error().Err(err).Msg("build TradeStart")
		return false
	}
	live.SendFrame(frame)
	return true
}

func (l *GameClientLink) sendTradeConfirmed(confirmer, partner *livePlayer) {
	partner.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1ConfirmedTrade, confirmer.Name))
	confirmer.SendFrame(serverpackets.FrameTradePressOwnOk())
	partner.SendFrame(serverpackets.FrameTradePressOtherOk())
}

func (l *GameClientLink) livePlayerByID(objectID int32) (*livePlayer, bool) {
	if l.world == nil {
		return nil, false
	}
	obj, ok := l.world.Player(objectID)
	if !ok {
		return nil, false
	}
	live, ok := obj.(*livePlayer)
	return live, ok
}

func (l *GameClientLink) validTradeParticipants(first, second *livePlayer) bool {
	if first == nil || second == nil || first.Inventory() == nil || second.Inventory() == nil {
		return false
	}
	if current, ok := l.livePlayerByID(first.ObjectID()); !ok || current != first {
		return false
	}
	if current, ok := l.livePlayerByID(second.ObjectID()); !ok || current != second {
		return false
	}
	return livePlayersInRange(first, second, tradeInteractionDistance)
}

func livePlayersInRange(first, second *livePlayer, radius int) bool {
	ax, ay, az := first.Position()
	bx, by, bz := second.Position()
	return location.In3DRange(ax, ay, az, bx, by, bz, radius)
}

func (l *GameClientLink) tradeParticipants(session tradebook.Session) (*livePlayer, *livePlayer, bool) {
	first, firstOK := l.livePlayerByID(session.FirstID)
	second, secondOK := l.livePlayerByID(session.SecondID)
	return first, second, firstOK && secondOK
}

func (l *GameClientLink) transferTradeInventoryItem(ctx context.Context, source, receiver *itemcontainer.Inventory, objectID int32, count int) bool {
	res, ok, err := l.inventoryService().TransferItem(source, receiver, objectID, count)
	if err != nil {
		l.log.Error().Err(err).Msg("allocate trade item id")
		return false
	}
	if ok {
		l.applyPersistActions(ctx, res.Persist)
	}
	return ok
}

func tradeSettlementMessage(status tradebook.SettlementStatus) int {
	switch status {
	case tradebook.SettlementWeightExceeded:
		return serverpackets.SystemMessageWeightLimitExceeded
	case tradebook.SettlementSlotsFull:
		return serverpackets.SystemMessageSlotsFull
	default:
		return 0
	}
}

func tradeItemSnapshot(item tradebook.ItemSnapshot) serverpackets.TradeItemSnapshot {
	return serverpackets.TradeItemSnapshot{
		ObjectID:     item.ObjectID,
		TemplateID:   item.TemplateID,
		Count:        item.Count,
		EnchantLevel: item.EnchantLevel,
	}
}

func tradeItemUpdateEntries(entries []tradebook.ItemUpdateEntry) []serverpackets.TradeItemUpdateEntry {
	out := make([]serverpackets.TradeItemUpdateEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, serverpackets.TradeItemUpdateEntry{
			Item:           tradeItemSnapshot(entry.Item),
			AvailableCount: entry.AvailableCount,
		})
	}
	return out
}
