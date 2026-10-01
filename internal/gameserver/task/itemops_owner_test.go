package task

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
)

// handlerWrite does what a handler's write does inside its operation once it
// has mutated own: widens to every row bound to them, binds them all, and
// takes their places with the states it reads.
func (l *tradeLegs) handlerWrite(own ...*item.Instance) (*persist.Write, BoundGroups, item.FlushBatch) {
	ids := make([]int32, 0, len(own))
	rows := make([]BoundRow, 0, len(own))
	for _, inst := range own {
		ids = append(ids, inst.ObjectID)
		rows = append(rows, BoundRow{ObjectID: inst.ObjectID, OwnerID: inst.Snapshot().OwnerID, Inst: inst})
	}
	widened, _ := l.instances.Widen(ids)
	rows = append(rows, widened...)
	carried := l.instances.Bind(rows)
	write := l.instances.writes.Begin()
	var batch item.FlushBatch
	for _, row := range rows {
		row.Inst.WithState(func(st item.InstanceState) {
			write.Add(st.ObjectID)
			batch.Saves = append(batch.Saves, st)
		})
	}
	return write, carried, batch
}

// waitParkedOrDone polls until one operation is parked on the gate or done
// closes, whichever comes first.
func waitParkedOrDone(t *testing.T, instances *ItemInstances, done <-chan struct{}) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if ops, _ := instances.ops.parked(); ops == 1 {
			return
		}
		select {
		case <-done:
			return
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("the operation neither parked nor ended")
		}
		time.Sleep(time.Millisecond)
	}
}

// waitDone fails unless done closes within a few seconds.
func waitDone(t *testing.T, done <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("%s waited", what)
	}
}

// TestItemInstancesPartnerWriteOnReceivedLegWaitsForTrade pins #3128: the
// partner's own item operation on the leg a trade just gave it — spending 5
// of the 10 units it received — cannot run its write between the trade's
// mutation and its binding. Run there, the write would find no group, take
// the receiver's leg alone and land it ahead of the trade's write, so a crash
// in between would leave the units in both inventories. The partner's
// operation holds the receiver's owner, which the trade holds until its legs
// are bound and placed, so it waits; its write then widens to the giver's leg
// and lands both.
func TestItemInstancesPartnerWriteOnReceivedLegWaitsForTrade(t *testing.T) {
	flusher := &chunkTrackingFlusher{}
	legs := newTradeLegs(flusher)

	endTrade := legs.instances.BeginOperation(100, 200)
	legs.mutate()

	partner := make(chan struct{})
	go func() {
		defer close(partner)
		end := legs.instances.BeginOperation(200)
		defer end()
		legs.receiver.ReduceCount(5)
		write, carried, batch := legs.handlerWrite(legs.receiver)
		end()
		legs.land(flusher, write, carried, batch)
	}()
	// The partner's operation parks on the trade's owners before it touches
	// the received leg. Had it not, it would have run to its end by now, and
	// its write would show here.
	waitParkedOrDone(t, legs.instances, partner)
	if calls := flusher.calls(); len(calls) != 0 {
		t.Fatalf("the partner's write landed %+v before the trade bound its legs", calls)
	}

	write, carried, batch := legs.bindAndReserve()
	endTrade()
	waitDone(t, partner, "the partner's operation, once the trade ended,")
	legs.land(flusher, write, carried, batch)

	calls := flusher.calls()
	for n, batch := range calls {
		var giver, receiver bool
		for _, st := range batch.Saves {
			switch {
			case st.ObjectID == 1 && st.Count == 50:
				giver = true
			case st.ObjectID == 2 && st.Count != 40:
				receiver = true
			}
		}
		if receiver && !giver {
			t.Fatalf("flush %d committed the received leg without the giver's: %+v", n, batch.Saves)
		}
	}
	// The partner's write took the later places on both legs and landed
	// first, carrying the giver's leg along with its own spend.
	first := calls[0].Saves
	if len(first) != 2 {
		t.Fatalf("partner's flush = %+v, want both legs", first)
	}
	for _, st := range first {
		if want := map[int32]int{1: 50, 2: 45}[st.ObjectID]; st.Count != want {
			t.Fatalf("partner's flush row %d count = %d, want %d", st.ObjectID, st.Count, want)
		}
	}
}

// TestItemInstancesOperationHoldsOwnersBoundToItsOwn pins the owners an
// operation holds beyond the ones it names: while a trade's write has not
// landed, a write of one leg widens to the other and reads it, so an
// operation on the receiver's owner holds the giver's too and waits for an
// operation the giver has open. Once the trade's write lands, the owners are
// apart again.
func TestItemInstancesOperationHoldsOwnersBoundToItsOwn(t *testing.T) {
	flusher := &chunkTrackingFlusher{}
	legs := newTradeLegs(flusher)

	endTrade := legs.instances.BeginOperation(100, 200)
	legs.mutate()
	write, carried, batch := legs.bindAndReserve()
	endTrade()

	endGiver := legs.instances.BeginOperation(100)
	partner := make(chan struct{})
	var heldGiver bool
	go func() {
		defer close(partner)
		end := legs.instances.BeginOperation(200)
		defer end()
		heldGiver = legs.instances.OperationHolds(100)
	}()
	waitParked(t, legs.instances, 1, 0)
	endGiver()
	waitDone(t, partner, "the receiver's operation, once the giver's ended,")
	if !heldGiver {
		t.Fatal("the receiver's operation did not hold the giver's owner while their legs were bound")
	}

	legs.land(flusher, write, carried, batch)
	end := legs.instances.BeginOperation(200)
	defer end()
	if legs.instances.OperationHolds(100) {
		t.Fatal("the receiver's operation still holds the giver's owner after the trade's write landed")
	}
}

// TestItemInstancesOperationsOnOtherOwnersDoNotWait pins that owners keep
// operations apart only when they share one: two players' operations on their
// own inventories run at once.
func TestItemInstancesOperationsOnOtherOwnersDoNotWait(t *testing.T) {
	legs := newTradeLegs(&chunkTrackingFlusher{})

	end := legs.instances.BeginOperation(100)
	defer end()
	other := make(chan struct{})
	go func() {
		defer close(other)
		legs.instances.BeginOperation(200)()
	}()
	waitDone(t, other, "an operation on another owner")
}

// TestItemInstancesNestedOperationNamesNoOwners pins that an operation opened
// inside another on the same goroutine, naming no owners, does not wait for
// the owners the outer one holds.
func TestItemInstancesNestedOperationNamesNoOwners(t *testing.T) {
	legs := newTradeLegs(&chunkTrackingFlusher{})

	outer := legs.instances.BeginOperation(100, 200)
	defer outer()
	inner := make(chan struct{})
	go func() {
		defer close(inner)
		legs.instances.BeginOperation()()
	}()
	waitDone(t, inner, "the nested operation")
	if !legs.instances.OperationHolds(100) || !legs.instances.OperationHolds(200) {
		t.Fatal("the nested operation's end released the outer one's owners")
	}
}
