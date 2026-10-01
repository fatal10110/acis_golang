package task

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// TestItemInstancesPickupOfBoundGroundItemWaitsForGroupOwners pins the
// ground-item path of #3128. A trade gave the receiver its stack and the
// trade's write has not landed, so the stack is still bound to the giver's
// leg. The receiver drops the stack — no item operation — and a third player
// picks it up while the giver sells to a buy store. The pickup's write widens
// from the ground row to the giver's leg; run in the middle of the sale, it
// would read the giver's row after the sale changed it and before the sale
// bound it, and land it without the store owner's leg. The pickup names the
// ground row it takes, so it holds the owners of that row's group and waits
// for the sale.
func TestItemInstancesPickupOfBoundGroundItemWaitsForGroupOwners(t *testing.T) {
	const picker, storeOwner int32 = 300, 400
	flusher := &chunkTrackingFlusher{}
	legs := newTradeLegs(flusher)
	store := &item.Instance{ObjectID: 4, TemplateID: 1, OwnerID: storeOwner, Count: 10, Location: item.LocationInventory}
	store.BindPersister(ownerPersister{instances: legs.instances, ownerID: storeOwner})

	endTrade := legs.instances.BeginOperation(100, 200)
	legs.mutate()
	tradeWrite, tradeCarried, tradeBatch := legs.bindAndReserve()
	endTrade()
	// The receiver drops the stack it was just given: the row leaves its
	// inventory outside any operation and keeps its binding.
	legs.receiver.SetOwner(0)

	// The giver sells 5 of its 50 to the store owner's buy store.
	endSale := legs.instances.BeginOperation(100, storeOwner)
	legs.giver.ReduceCount(5)
	store.AddCount(5)

	pickup := make(chan struct{})
	go func() {
		defer close(pickup)
		end := legs.instances.BeginOperationTaking([]int32{legs.receiver.ObjectID}, picker)
		defer end()
		legs.receiver.SetOwner(picker)
		write, carried, batch := legs.handlerWrite(legs.receiver)
		end()
		legs.land(flusher, write, carried, batch)
	}()
	waitParkedOrDone(t, legs.instances, pickup)
	if calls := flusher.calls(); len(calls) != 0 {
		t.Fatalf("the pickup's write landed %+v in the middle of the giver's sale", calls)
	}

	saleWrite, saleCarried, saleBatch := legs.handlerWrite(legs.giver, store)
	endSale()
	waitDone(t, pickup, "the pickup, once the sale ended,")
	legs.land(flusher, saleWrite, saleCarried, saleBatch)
	legs.land(flusher, tradeWrite, tradeCarried, tradeBatch)

	for n, batch := range flusher.calls() {
		var sold, paid bool
		for _, st := range batch.Saves {
			switch {
			case st.ObjectID == legs.giver.ObjectID && st.Count == 45:
				sold = true
			case st.ObjectID == store.ObjectID && st.Count == 15:
				paid = true
			}
		}
		if sold != paid {
			t.Fatalf("flush %d committed one leg of the sale alone: %+v", n, batch.Saves)
		}
	}
}

// TestItemInstancesOperationHoldsOwnersOfRowsMovedSinceBound pins the
// current owner's side of the closure: a bound row that changed hands outside
// an operation after its binding is written by its new owner's operations,
// whose writes widen to its group, so they hold the group's owners too.
func TestItemInstancesOperationHoldsOwnersOfRowsMovedSinceBound(t *testing.T) {
	const newOwner int32 = 300
	legs := newTradeLegs(&chunkTrackingFlusher{})

	endTrade := legs.instances.BeginOperation(100, 200)
	legs.mutate()
	legs.bindAndReserve()
	endTrade()
	legs.receiver.SetOwner(newOwner)

	end := legs.instances.BeginOperation(newOwner)
	defer end()
	if !legs.instances.OperationHolds(100) || !legs.instances.OperationHolds(200) {
		t.Fatal("an operation on the row's new owner does not hold the owners of the group it is still bound to")
	}
}

// TestItemInstancesTakenRowOutsideGroupsHoldsOnlyOwners pins that naming a
// taken row bound to nothing adds no owner.
func TestItemInstancesTakenRowOutsideGroupsHoldsOnlyOwners(t *testing.T) {
	legs := newTradeLegs(&chunkTrackingFlusher{})

	end := legs.instances.BeginOperationTaking([]int32{legs.receiver.ObjectID}, 300)
	defer end()
	if !legs.instances.OperationHolds(300) || legs.instances.OperationHolds(200) || legs.instances.OperationHolds(100) {
		t.Fatal("an operation taking an unbound row held owners beyond its own")
	}
}
