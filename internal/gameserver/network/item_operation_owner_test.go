package network

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
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

// TestPickupOfBoundGroundItemHoldsTheGroupsOwners pins the pickup's wiring
// of the ground-item path of #3128: the ground item's row is still bound to
// a leg of another owner — the trade that gave it to its dropper has not
// landed — and the pickup's write widens to that leg, so the pickup holds
// that owner from the merge to the write. An operation of that owner meanwhile
// (a sale changing the leg) would otherwise have the pickup read the leg
// changed but not bound, and land it without the sale's other leg.
func TestPickupOfBoundGroundItemHoldsTheGroupsOwners(t *testing.T) {
	link, _, _, _, first, second := newDirectTradeFixture(t)
	probe := newGateProbe(link)
	link.groundItems = task.NewGroundItems(link.world, task.DefaultGroundItemOptions(), nil)

	const heldStack, groundObject, giverStack, giver int32 = 5700, 5701, 5702, 7700
	addProbedStack(link, probe, first.ObjectID(), first.Inventory().AddNew(item.AdenaID, 40, heldStack))
	tmpl, ok := link.itemTemplates.Get(item.AdenaID)
	if !ok {
		t.Fatal("missing adena template")
	}
	ground, err := grounditem.New(item.Instance{ObjectID: groundObject, TemplateID: item.AdenaID, Count: 10, Location: item.LocationVoid}, tmpl)
	if err != nil {
		t.Fatalf("grounditem.New: %v", err)
	}
	giverLeg := &item.Instance{ObjectID: giverStack, TemplateID: item.AdenaID, OwnerID: giver, Count: 50, Location: item.LocationInventory}
	probe.instances.Bind([]task.BoundRow{
		{ObjectID: groundObject, OwnerID: second.ObjectID(), Inst: &ground.Instance},
		{ObjectID: giverStack, OwnerID: giver, Inst: giverLeg},
	})
	probe.covered = func() bool {
		return probe.instances.OperationHolds(first.ObjectID()) && probe.instances.OperationHolds(giver)
	}
	x, y, z := first.Position()
	link.world.Spawn(ground, x, y, z, 0)

	probe.start()
	if !link.pickupLiveGroundItem(context.Background(), first, ground) {
		t.Fatal("pickupLiveGroundItem did not handle the ground item")
	}

	if got := first.Inventory().ItemByObjectID(heldStack); got == nil || got.CountValue() != 50 {
		t.Fatalf("held stack after the pickup = %+v, want 50", got)
	}
	probe.assertCovered(t)
	if probe.instances.OperationHolds(giver) {
		t.Fatal("the pickup kept the giver's owner: every later operation of the giver would wait forever")
	}
}
