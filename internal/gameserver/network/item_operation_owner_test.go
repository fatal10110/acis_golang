package network

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	tradebook "github.com/fatal10110/acis_golang/internal/gameserver/trade"
)

// TestSettleConfirmedTradeHoldsBothTradersOwners pins the trade's wiring of
// #3128: its operation holds both traders' owners from the exchange to the
// write, so neither trader's own item operation — the partner spending what
// it just received, on its own queue — can write one leg between the
// exchange and the trade's binding.
func TestSettleConfirmedTradeHoldsBothTradersOwners(t *testing.T) {
	link, _, _, _, first, second := newDirectTradeFixture(t)
	probe := newGateProbe(link)
	probe.covered = func() bool {
		return probe.instances.OperationHolds(first.ObjectID()) && probe.instances.OperationHolds(second.ObjectID())
	}

	const giverStack, receiverStack int32 = 5600, 5601
	addProbedStack(link, probe, first.ObjectID(), first.Inventory().AddNew(item.AdenaID, 60, giverStack))
	addProbedStack(link, probe, second.ObjectID(), second.Inventory().AddNew(item.AdenaID, 40, receiverStack))

	link.handleTradeRequest(first, clientpackets.TradeRequest{ObjectID: second.ObjectID()})
	link.handleAnswerTradeRequest(second, clientpackets.AnswerTradeRequest{Response: 1})
	link.handleAddTradeItem(first, clientpackets.AddTradeItem{ObjectID: giverStack, Count: 10})
	link.handleTradeDone(context.Background(), second, clientpackets.TradeDone{Response: 1})
	ready := link.trades.Confirm(first.ObjectID())
	if ready.Status != tradebook.DoneReady {
		t.Fatalf("Confirm status = %v, want ready", ready.Status)
	}

	probe.start()
	link.settleConfirmedTrade(ready.Session, first.ObjectID())

	if got := second.Inventory().ItemByObjectID(receiverStack); got == nil || got.CountValue() != 50 {
		t.Fatalf("receiver stack after the trade = %+v, want 50", got)
	}
	probe.assertCovered(t)
	if probe.instances.OperationHolds(first.ObjectID()) || probe.instances.OperationHolds(second.ObjectID()) {
		t.Fatal("the settlement kept its owners: every later operation on either trader would wait forever")
	}
}
