package task

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
)

// tradeLegs is a trade of 10 units between two owners' stacks: the giver's
// stack (row 1, owner 100) holds 60 before it and 50 after, the receiver's
// (row 2, owner 200) 40 before and 50 after.
type tradeLegs struct {
	instances       *ItemInstances
	giver, receiver *item.Instance
}

func newTradeLegs(flusher ItemFlusher) *tradeLegs {
	instances := NewItemInstances(flusher, item.NewTable(nil), nil, persist.NewOrder(), zerolog.Nop())
	legs := &tradeLegs{
		instances: instances,
		giver:     &item.Instance{ObjectID: 1, TemplateID: 1, OwnerID: 100, Count: 60, Location: item.LocationInventory},
		receiver:  &item.Instance{ObjectID: 2, TemplateID: 1, OwnerID: 200, Count: 40, Location: item.LocationInventory},
	}
	legs.giver.BindPersister(ownerPersister{instances: instances, ownerID: 100})
	legs.receiver.BindPersister(ownerPersister{instances: instances, ownerID: 200})
	return legs
}

// mutate moves the units, which the persister hooks put in the pending set.
func (l *tradeLegs) mutate() {
	l.giver.ReduceCount(10)
	l.receiver.AddCount(10)
}

// bindAndReserve does what a handler's write does once the operation has
// mutated: binds both rows and takes their places with the states it read.
func (l *tradeLegs) bindAndReserve() (*persist.Write, BoundGroups, item.FlushBatch) {
	carried := l.instances.Bind([]BoundRow{
		{ObjectID: 1, OwnerID: 100, Inst: l.giver},
		{ObjectID: 2, OwnerID: 200, Inst: l.receiver},
	})
	write := l.instances.writes.Begin()
	var batch item.FlushBatch
	for _, inst := range []*item.Instance{l.giver, l.receiver} {
		inst.WithState(func(st item.InstanceState) {
			write.Add(st.ObjectID)
			batch.Saves = append(batch.Saves, st)
		})
	}
	return write, carried, batch
}

// land runs the operation's own write, as its queued job would.
func (l *tradeLegs) land(flusher ItemFlusher, write *persist.Write, carried BoundGroups, batch item.FlushBatch) {
	write.Run(func(keep []int32) error {
		kept := item.FlushBatch{Saves: slices.DeleteFunc(slices.Clone(batch.Saves), func(st item.InstanceState) bool {
			return !slices.Contains(keep, st.ObjectID)
		})}
		if err := flusher.Flush(context.Background(), kept); err != nil {
			return err
		}
		l.instances.Landed(carried, keep)
		return nil
	})
}

// assertNoSingleLeg fails when a flush commits the trade's state of one leg
// without the other: a crash right after that commit would leave the units
// in both inventories or in neither. Writes from before the trade, and writes
// once both legs have landed together, are fine.
func assertNoSingleLeg(t *testing.T, calls []item.FlushBatch) {
	t.Helper()
	traded := map[int32]int{1: 50, 2: 50}
	for n, batch := range calls {
		var legs int
		for _, st := range batch.Saves {
			if want, ok := traded[st.ObjectID]; ok && st.Count == want {
				legs++
			}
		}
		if legs == 1 {
			t.Fatalf("flush %d committed one traded leg alone: %+v", n, batch.Saves)
		}
		if legs == 2 {
			return
		}
	}
	t.Fatalf("no flush committed the traded legs together: %+v", calls)
}

// waitParked polls until the gate has the given operations and writes
// parked on it.
func waitParked(t *testing.T, instances *ItemInstances, ops, reads int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		gotOps, gotReads := instances.ops.parked()
		if gotOps == ops && gotReads == reads {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("gate parked = %d operations, %d writes; want %d, %d", gotOps, gotReads, ops, reads)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestItemInstancesSaveWaitsForOperationToBind pins the window between an
// operation's mutation and its binding: a tick that swaps the mutated legs
// out of pending in that window must not read either of them until the
// operation has bound them, so its write carries both or neither.
func TestItemInstancesSaveWaitsForOperationToBind(t *testing.T) {
	flusher := &chunkTrackingFlusher{}
	legs := newTradeLegs(flusher)

	end := legs.instances.BeginOperation()
	legs.mutate()

	saved := make(chan error, 1)
	go func() { saved <- legs.instances.Save(context.Background()) }()
	// The tick has swapped both legs out of pending and its first owner job
	// is waiting to read its leg.
	waitParked(t, legs.instances, 0, 1)
	if calls := flusher.calls(); len(calls) != 0 {
		t.Fatalf("tick flushed %+v while the operation was open", calls)
	}

	write, carried, batch := legs.bindAndReserve()
	end()
	if err := <-saved; err != nil {
		t.Fatalf("Save: %v", err)
	}
	legs.land(flusher, write, carried, batch)

	calls := flusher.calls()
	assertNoSingleLeg(t, calls)
	if got := batchRowIDs(calls[0]); !slices.Equal(got, []int32{1, 2}) {
		t.Fatalf("tick's first flush rows = %v, want both bound legs [1 2]", got)
	}
}

// TestItemInstancesOperationWaitsForReadingWrite pins the window between a
// write's Widen and its state reads: an operation that mutated, bound and
// took its places inside it would leave that write, which saw no group,
// holding the later place on its leg, and the operation's own write would
// then land the other leg alone. The operation instead waits for the write
// to finish reading, which reads the leg as it was before the trade.
func TestItemInstancesOperationWaitsForReadingWrite(t *testing.T) {
	flusher := &chunkTrackingFlusher{}
	legs := newTradeLegs(flusher)

	operated := make(chan struct{})
	legs.instances.afterWiden = func() {
		legs.instances.afterWiden = nil
		go func() {
			defer close(operated)
			end := legs.instances.BeginOperation()
			defer end()
			legs.mutate()
			write, carried, batch := legs.bindAndReserve()
			end()
			legs.land(flusher, write, carried, batch)
		}()
		// The operation is parked on the gate, not yet mutating.
		waitParked(t, legs.instances, 1, 0)
	}

	if err := legs.instances.UpdateItems(context.Background(), []*item.Instance{legs.giver}); err != nil {
		t.Fatalf("UpdateItems: %v", err)
	}
	<-operated

	// Which write takes the rows first is up to the scheduler: the trade's,
	// holding the later places, makes the tick's skip them. Either way the
	// tick read the giver's leg before the trade, and the trade's legs land
	// together.
	calls := flusher.calls()
	assertNoSingleLeg(t, calls)
	for _, batch := range calls[:len(calls)-1] {
		if got := batch.Saves; len(got) != 1 || got[0].ObjectID != 1 || got[0].Count != 60 {
			t.Fatalf("tick's flush = %+v, want the giver's leg as it was before the trade", got)
		}
	}
	if got := batchRowIDs(calls[len(calls)-1]); !slices.Equal(got, []int32{1, 2}) {
		t.Fatalf("trade's flush rows = %v, want both legs [1 2]", got)
	}
}

// TestItemInstancesNestedOperationPassesParkedWrite pins that an operation
// opened inside another does not wait for a write parked behind the outer
// one, which would wait for the outer operation in turn.
func TestItemInstancesNestedOperationPassesParkedWrite(t *testing.T) {
	flusher := &chunkTrackingFlusher{}
	legs := newTradeLegs(flusher)

	outer := legs.instances.BeginOperation()
	defer outer()
	written := make(chan error, 1)
	go func() {
		written <- legs.instances.UpdateItems(context.Background(), []*item.Instance{legs.giver})
	}()
	waitParked(t, legs.instances, 0, 1)

	inner := make(chan struct{})
	go func() {
		defer close(inner)
		legs.instances.BeginOperation()()
	}()
	select {
	case <-inner:
	case <-time.After(10 * time.Second):
		t.Fatal("nested operation waited for the parked write")
	}

	outer()
	if err := <-written; err != nil {
		t.Fatalf("UpdateItems: %v", err)
	}
	if calls := flusher.calls(); len(calls) != 1 {
		t.Fatalf("flushes = %+v, want the parked write once the operation ended", calls)
	}
}
