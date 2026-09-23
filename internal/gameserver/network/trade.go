package network

import (
	"context"
	"time"

	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
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
		// The session exists but has no partner recorded — equivalent
		// to the reference's getPartner() == null, which the reference
		// answers with TARGET_IS_NOT_FOUND and a trade cancel. The race
		// is rare (partner logged off between the two packets) but a
		// silent return would leave the trader's add-item click pending
		// forever.
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetNotFound))
		l.cancelTradeByID(live.ObjectID())
		return
	}
	partner, ok := l.livePlayerByID(partnerID)
	if !ok || !l.tradePartnerLive(live, partner) {
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
		// Same race shape as handleAddTradeItem: the session is here but
		// its partner is gone, which the reference answers with
		// TARGET_IS_NOT_FOUND. Answer the same instead of dropping the
		// confirm packet silently.
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetNotFound))
		return
	}
	partner, ok := l.livePlayerByID(partnerID)
	if !ok || !l.tradePartnerLive(live, partner) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetNotFound))
		return
	}
	if !livePlayersInRange(live, partner, tradeInteractionDistance) {
		// The reference validates the interaction radius on every confirm
		// and answers an out-of-range confirm by cancelling the whole
		// trade for both players, not with a per-player error message.
		l.cancelTradeByID(live.ObjectID())
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

	l.settleConfirmedTrade(result.Session, live.ObjectID())
}

// settleConfirmedTrade exchanges the offers of a session both sides have
// confirmed. The reference re-validates at settlement too (TradeList.confirm,
// both-sides-confirmed branch) and answers a failed re-check by cancelling
// the whole trade for both players — the same cancel broadcast as everywhere
// else — not with the exchange-ended finish of a failed transfer.
//
// The partner's own queue keeps running while this settles on the
// confirmer's, so the check and every move happen inside one exchange that
// holds both inventories: nothing the partner does can slip between them and
// leave the trade half done. Both inventories' updates and weight reach their
// owners through the inventory-update tick, on each owner's own queue.
func (l *GameClientLink) settleConfirmedTrade(session tradebook.Session, confirmerID int32) {
	// Confirm already took the ready session out of the book, so a failed
	// re-check cancels straight to the participants: a book cancel would
	// reach no one.
	first, second, ok := l.tradeParticipants(session)
	if !ok {
		// A participant left the world after Confirm. The reference answers
		// a partner gone offline with the confirmer's own cancel, which
		// names the confirmer; the one who left gets nothing.
		for _, live := range []*livePlayer{first, second} {
			if live != nil && live.ObjectID() == confirmerID {
				sendTradeCancel(live, live.Name)
			}
		}
		return
	}
	if !l.validTradeParticipants(first, second) {
		sendTradeCanceled(first, second)
		return
	}

	status := tradebook.SettlementEmpty
	if !session.Empty() {
		res, moved, err := l.inventory.Exchange(first.Inventory(), second.Inventory(),
			tradeMoves(session.FirstOffer), tradeMoves(session.SecondOffer),
			func(first, second itemcontainer.Held) bool {
				status = session.Check(first, second)
				return status == tradebook.SettlementOK
			})
		switch {
		case err != nil:
			l.log.Error().Err(err).Msg("allocate trade item id")
			status = tradebook.SettlementTransferFailed
		case !moved && status == tradebook.SettlementOK:
			status = tradebook.SettlementTransferFailed
		}
		l.applyPersistActions(res.Persist)
	}
	if status == tradebook.SettlementInvalidItems {
		sendTradeCanceled(first, second)
		return
	}
	failMessage := tradeSettlementMessage(status)
	if failMessage != 0 {
		first.SendFrame(serverpackets.FrameSystemMessage(failMessage))
		second.SendFrame(serverpackets.FrameSystemMessage(failMessage))
	}
	l.finishTrade(first, second, status == tradebook.SettlementOK)
}

func tradeMoves(offer tradebook.Offer) []invops.Move {
	moves := make([]invops.Move, 0, len(offer.Items))
	for _, row := range offer.Items {
		moves = append(moves, invops.Move{ObjectID: row.Snapshot.ObjectID, Count: row.Count})
	}
	return moves
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
	sendTradeCanceled(first, second)
}

func sendTradeCanceled(first, second *livePlayer) {
	sendTradeCancel(first, second.Name)
	sendTradeCancel(second, first.Name)
}

func sendTradeCancel(live *livePlayer, cancellerName string) {
	live.SendFrame(serverpackets.FrameSendTradeDone(false))
	live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1CanceledTrade, cancellerName))
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
	return l.tradePartnerLive(first, second) && livePlayersInRange(first, second, tradeInteractionDistance)
}

// tradePartnerLive reports whether both players are still online in the
// world, without the 150-unit interaction-distance check. AddTradeItem.java
// L40-46 gates on partner liveness only (partner != null, still in World,
// getActiveTradeList() != null) — the reference enforces
// Npc.INTERACTION_DISTANCE solely in TradeList.validate(), called from
// TradeList.confirm() (TradeList.java L326-341), i.e. only at TradeDone.
func (l *GameClientLink) tradePartnerLive(first, second *livePlayer) bool {
	if first == nil || second == nil || first.Inventory() == nil || second.Inventory() == nil {
		return false
	}
	if current, ok := l.livePlayerByID(first.ObjectID()); !ok || current != first {
		return false
	}
	if current, ok := l.livePlayerByID(second.ObjectID()); !ok || current != second {
		return false
	}
	return true
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
