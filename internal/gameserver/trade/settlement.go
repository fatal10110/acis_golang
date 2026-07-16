package trade

import "github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"

// Settle validates and exchanges both offers in a ready direct-trade session.
func Settle(session Session, firstInv, secondInv *itemcontainer.Inventory, transfer func(source, receiver *itemcontainer.Inventory, objectID int32, count int) bool) SettlementResult {
	if session.Empty() {
		return SettlementResult{Status: SettlementEmpty}
	}
	if !session.ValidItems(firstInv, secondInv) {
		return SettlementResult{Status: SettlementInvalidItems}
	}
	switch session.ReceiverStatus(firstInv, secondInv) {
	case ReceiverWeightExceeded:
		return SettlementResult{Status: SettlementWeightExceeded}
	case ReceiverSlotsFull:
		return SettlementResult{Status: SettlementSlotsFull}
	}
	if !settleOffer(firstInv, secondInv, session.FirstOffer, transfer) {
		return SettlementResult{Status: SettlementTransferFailed}
	}
	if !settleOffer(secondInv, firstInv, session.SecondOffer, transfer) {
		return SettlementResult{Status: SettlementTransferFailed}
	}
	return SettlementResult{Status: SettlementOK}
}

func settleOffer(source, receiver *itemcontainer.Inventory, offer Offer, transfer func(source, receiver *itemcontainer.Inventory, objectID int32, count int) bool) bool {
	if source == nil || receiver == nil || transfer == nil {
		return false
	}
	for _, row := range offer.Items {
		if !transfer(source, receiver, row.Snapshot.ObjectID, row.Count) {
			return false
		}
	}
	source.UpdateWeight()
	receiver.UpdateWeight()
	return true
}
