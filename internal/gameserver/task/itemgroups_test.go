package task

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
)

func batchRowIDs(batch item.FlushBatch) []int32 {
	ids := make([]int32, 0, len(batch.Saves)+len(batch.Deletes))
	for _, s := range batch.Saves {
		ids = append(ids, s.ObjectID)
	}
	ids = append(ids, batch.Deletes...)
	slices.Sort(ids)
	return ids
}

// TestItemInstancesSaveWritesBoundRowsTogether pins that the tick writes rows
// bound across owners in one Flush, and that once such a write lands the rows
// are written apart again.
func TestItemInstancesSaveWritesBoundRowsTogether(t *testing.T) {
	flusher := &chunkTrackingFlusher{}
	instances := NewItemInstances(flusher, item.NewTable(nil), nil, nil, zerolog.Nop())
	giver := &item.Instance{ObjectID: 1, TemplateID: 1, OwnerID: 100, Count: 60, Location: item.LocationInventory}
	receiver := &item.Instance{ObjectID: 2, TemplateID: 1, OwnerID: 200, Count: 40, Location: item.LocationInventory}
	instances.Add(giver)
	instances.Add(receiver)
	instances.Bind([]BoundRow{{ObjectID: 1, OwnerID: 100, Inst: giver}, {ObjectID: 2, OwnerID: 200, Inst: receiver}})

	if err := instances.Save(context.Background()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// The giver's job runs first and carries the receiver's row along; the
	// receiver's own job then finds the group settled and writes its row
	// again on its own.
	calls := flusher.calls()
	if len(calls) == 0 {
		t.Fatal("Save flushed nothing")
	}
	if got := batchRowIDs(calls[0]); !slices.Equal(got, []int32{1, 2}) {
		t.Fatalf("first flush rows = %v, want both bound rows [1 2]", got)
	}

	if err := instances.UpdateItems(context.Background(), []*item.Instance{giver}); err != nil {
		t.Fatalf("UpdateItems: %v", err)
	}
	calls = flusher.calls()
	if got := batchRowIDs(calls[len(calls)-1]); !slices.Equal(got, []int32{1}) {
		t.Fatalf("write after the bound rows landed = %v, want only [1]", got)
	}
}

// TestItemInstancesBindMergesAndLandsWhole pins the group bookkeeping: binding
// a row already bound merges both groups, a deleted row with no instance is
// widened as a bare delete, and only a write that carried the group and wrote
// every row of it settles the group.
func TestItemInstancesBindMergesAndLandsWhole(t *testing.T) {
	instances := NewItemInstances(nil, item.NewTable(nil), nil, nil, zerolog.Nop())
	one := &item.Instance{ObjectID: 1, OwnerID: 100}
	two := &item.Instance{ObjectID: 2, OwnerID: 200}
	instances.Bind([]BoundRow{{ObjectID: 1, OwnerID: 100, Inst: one}, {ObjectID: 2, OwnerID: 200, Inst: two}})
	merged := instances.Bind([]BoundRow{{ObjectID: 2, OwnerID: 200, Inst: two}, {ObjectID: 3, OwnerID: 200}})

	widened, carried := instances.Widen([]int32{1})
	ids := make([]int32, 0, len(widened))
	for _, row := range widened {
		ids = append(ids, row.ObjectID)
		if row.ObjectID == 3 && row.Inst != nil {
			t.Fatalf("row 3 widened with instance %+v, want a bare delete", row.Inst)
		}
	}
	slices.Sort(ids)
	if !slices.Equal(ids, []int32{2, 3}) {
		t.Fatalf("Widen([1]) = %v, want [2 3]", ids)
	}
	if !slices.Equal(carried, merged) {
		t.Fatalf("Widen([1]) carried %v, want the merged group %v", carried, merged)
	}

	instances.Landed(merged, []int32{1, 2})
	if got, _ := instances.Widen([]int32{1}); len(got) != 2 {
		t.Fatalf("Widen after a partial landing = %+v, want the group intact", got)
	}
	instances.Landed(merged, []int32{1, 2, 3})
	if got, _ := instances.Widen([]int32{1, 2, 3}); len(got) != 0 {
		t.Fatalf("Widen after the whole group landed = %+v, want nothing", got)
	}
}

// TestItemInstancesLandedSettlesOnlyCarriedGroup pins the sequence where two
// operations bind the same rows and the first one's write lands after the
// second bound: that landing must leave the second group binding the rows.
func TestItemInstancesLandedSettlesOnlyCarriedGroup(t *testing.T) {
	instances := NewItemInstances(nil, item.NewTable(nil), nil, nil, zerolog.Nop())
	giver := &item.Instance{ObjectID: 1, OwnerID: 100}
	receiver := &item.Instance{ObjectID: 2, OwnerID: 200}
	trade := []BoundRow{{ObjectID: 1, OwnerID: 100, Inst: giver}, {ObjectID: 2, OwnerID: 200, Inst: receiver}}
	first := instances.Bind(trade)
	second := instances.Bind(trade)

	instances.Landed(first, []int32{1, 2})
	if got, _ := instances.Widen([]int32{2}); len(got) != 1 || got[0].ObjectID != 1 {
		t.Fatalf("Widen([2]) after the first trade's write landed = %+v, want the second trade's row 1", got)
	}
	instances.Landed(second, []int32{1, 2})
	if got, _ := instances.Widen([]int32{2}); len(got) != 0 {
		t.Fatalf("Widen([2]) after the second trade's write landed = %+v, want nothing", got)
	}
}

// hookFlusher records every batch and runs onFlush during the first Flush,
// while the write is in flight.
type hookFlusher struct {
	chunkTrackingFlusher
	onFlush func()
}

func (f *hookFlusher) Flush(ctx context.Context, batch item.FlushBatch) error {
	if hook := f.onFlush; hook != nil {
		f.onFlush = nil
		hook()
	}
	return f.chunkTrackingFlusher.Flush(ctx, batch)
}

// TestItemInstancesOlderWriteLandingKeepsNewerGroup pins that a write which
// widened to an operation's rows settles only the group it carried: a second
// operation that binds the same rows while that write is in flight keeps its
// own group until a write carrying it lands. Settling by row would let the
// older write clear the newer group, and a failure of the newer operation's
// own write would then leave its two rows to be written apart.
func TestItemInstancesOlderWriteLandingKeepsNewerGroup(t *testing.T) {
	flusher := &hookFlusher{}
	instances := NewItemInstances(flusher, item.NewTable(nil), nil, nil, zerolog.Nop())
	giver := &item.Instance{ObjectID: 1, TemplateID: 1, OwnerID: 100, Count: 60, Location: item.LocationInventory}
	receiver := &item.Instance{ObjectID: 2, TemplateID: 1, OwnerID: 200, Count: 40, Location: item.LocationInventory}
	instances.Add(giver)
	instances.Add(receiver)
	trade := []BoundRow{{ObjectID: 1, OwnerID: 100, Inst: giver}, {ObjectID: 2, OwnerID: 200, Inst: receiver}}
	instances.Bind(trade)
	// The second trade on the same two stacks binds while the first one's
	// rows are being written.
	flusher.onFlush = func() { instances.Bind(trade) }

	if err := instances.UpdateItems(context.Background(), []*item.Instance{giver}); err != nil {
		t.Fatalf("UpdateItems: %v", err)
	}
	if got := batchRowIDs(flusher.calls()[0]); !slices.Equal(got, []int32{1, 2}) {
		t.Fatalf("first write rows = %v, want both bound rows [1 2]", got)
	}

	if err := instances.UpdateItems(context.Background(), []*item.Instance{giver}); err != nil {
		t.Fatalf("UpdateItems: %v", err)
	}
	calls := flusher.calls()
	if got := batchRowIDs(calls[len(calls)-1]); !slices.Equal(got, []int32{1, 2}) {
		t.Fatalf("write after the older write landed = %v, want the second trade's rows [1 2] still together", got)
	}
}

// TestItemInstancesFailedWriteDoesNotSplitBoundRows pins a write that holds
// an earlier place on one bound row and a later one on the other: the tick's
// owner jobs widen to the same group on two lanes at once, and their places
// can interleave that way. The write holding the later place on the
// receiver's row runs first and the database refuses it. It landed nothing,
// so the giver's job must still carry both rows; counting the refused write
// as landed would leave it keeping only the giver's row and committing that
// leg alone (#3125).
func TestItemInstancesFailedWriteDoesNotSplitBoundRows(t *testing.T) {
	flusher := &chunkTrackingFlusher{}
	order := persist.NewOrder()
	instances := NewItemInstances(flusher, item.NewTable(nil), nil, order, zerolog.Nop())
	giver := &item.Instance{ObjectID: 1, TemplateID: 1, OwnerID: 100, Count: 60, Location: item.LocationInventory}
	receiver := &item.Instance{ObjectID: 2, TemplateID: 1, OwnerID: 200, Count: 40, Location: item.LocationInventory}
	instances.Add(giver)
	instances.Add(receiver)
	instances.Bind([]BoundRow{{ObjectID: 1, OwnerID: 100, Inst: giver}, {ObjectID: 2, OwnerID: 200, Inst: receiver}})

	// An earlier write holds the giver's row, so the giver's job takes its
	// places and then waits for that row.
	earlier := order.Reserve(1)
	held, release := make(chan struct{}), make(chan struct{})
	go earlier.Run(func([]int32) error {
		close(held)
		<-release
		return nil
	})
	<-held
	saved := make(chan error, 1)
	go func() { saved <- instances.UpdateItems(context.Background(), []*item.Instance{giver}) }()
	deadline := time.Now().Add(5 * time.Second)
	for order.Reserved(2) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the giver's job never took its place on the receiver's row")
		}
		time.Sleep(time.Millisecond)
	}

	// The other job, later on the receiver's row, runs first and is refused.
	order.Reserve(2).Run(func([]int32) error { return errors.New("refused") })
	close(release)
	if err := <-saved; err != nil {
		t.Fatalf("UpdateItems: %v", err)
	}
	calls := flusher.calls()
	if len(calls) != 1 {
		t.Fatalf("flushes = %d, want the giver's job's one", len(calls))
	}
	if got := batchRowIDs(calls[0]); !slices.Equal(got, []int32{1, 2}) {
		t.Fatalf("giver's job wrote rows %v after the refused write, want both bound rows [1 2]", got)
	}
}
