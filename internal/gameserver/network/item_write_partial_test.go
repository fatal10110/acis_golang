package network

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
)

// batchRecordingItemStore records the rows of every batch a handler writes, so
// a test can tell which rows one multi-row write actually carried.
type batchRecordingItemStore struct {
	itemStore
	mu      sync.Mutex
	batches [][]int32
}

func (s *batchRecordingItemStore) WriteBatch(ctx context.Context, batch item.FlushBatch) error {
	ids := make([]int32, 0, len(batch.Saves)+len(batch.Deletes))
	for _, st := range batch.Saves {
		ids = append(ids, st.ObjectID)
	}
	ids = append(ids, batch.Deletes...)
	s.mu.Lock()
	s.batches = append(s.batches, ids)
	s.mu.Unlock()
	return s.itemStore.WriteBatch(ctx, batch)
}

func (s *batchRecordingItemStore) written() [][]int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.batches)
}

// TestMultiRowItemWriteSkipsOnlyTheRowsALaterWriteLanded pins the narrowing a
// multi-row write takes when it finally gets its rows: a row a later write has
// already landed is left out, and every other row is still written. A trade
// write carries both inventories' rows; if it wrote its whole batch after
// waiting, a row the new holder saved (or destroyed) in the meantime would be
// put back to the state the trade captured — the item handed back to the
// giver, or a destroyed stack restored.
//
// The two-row write is held off one of its rows by another lane's write of
// that row, the way the persistence tick's chunk holds a row for its whole
// transaction, so it keeps giving its lane back and re-queueing while the
// other row's later write lands.
func TestMultiRowItemWriteSkipsOnlyTheRowsALaterWriteLanded(t *testing.T) {
	link, store, _, _, first, second := newDirectTradeFixture(t)
	worker := persist.New(zerolog.Nop())
	t.Cleanup(func() {
		if err := worker.Close(context.Background()); err != nil {
			t.Errorf("close persistence worker: %v", err)
		}
	})
	order := persist.NewOrder()
	link.persist = worker
	link.itemWrites = order
	recording := &batchRecordingItemStore{itemStore: link.items}
	link.items = recording

	const adenaID, potionID int32 = 5400, 5401
	inv := first.Inventory()
	adena := inv.AddNew(item.AdenaID, 100, adenaID)
	potion := inv.AddNew(20, 3, potionID)
	inv.DrainUpdates()

	// Another lane holds the adena row, reserved ahead of the trade's write.
	// Owner 4 maps to a different lane than either trader.
	holder := order.Reserve(adenaID)
	held, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releaseRow := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseRow)
	worker.Enqueue(4, func() {
		holder.Run(func([]int32) error {
			close(held)
			<-release
			return nil
		})
	})
	<-held

	// The two-row write: both rows still the first trader's.
	link.applyPersistActions([]invops.Persist{invops.Save(adena), invops.Save(potion)})
	if got := order.Reserved(potionID); got != 1 {
		t.Fatalf("potion row places = %d, want the two-row write's 1", got)
	}

	// The potion then moves whole to the second trader, whose single-row write
	// is reserved after the two-row one and lands while that one waits.
	res, ok, err := link.inventory.TransferItem(inv, second.Inventory(), potionID, 3)
	if err != nil || !ok {
		t.Fatalf("TransferItem() = ok %v, err %v", ok, err)
	}
	link.applyPersistActions(res.Persist)
	if err := worker.Flush(context.Background(), second.ObjectID()); err != nil {
		t.Fatalf("flush receiver lane: %v", err)
	}
	if owner := persistedItemOwner(t, store, second.ObjectID(), potionID); owner != second.ObjectID() {
		t.Fatalf("potion owned by %d after the receiver's write, want %d", owner, second.ObjectID())
	}
	if got := recording.written(); len(got) != 1 || !slices.Equal(got[0], []int32{potionID}) {
		t.Fatalf("writes before the adena row is free = %v, want only the receiver's [%d]", got, potionID)
	}

	releaseRow()
	if err := worker.Flush(context.Background(), first.ObjectID(), second.ObjectID(), 4); err != nil {
		t.Fatalf("flush lanes: %v", err)
	}

	got := recording.written()
	if len(got) != 2 || !slices.Equal(got[1], []int32{adenaID}) {
		t.Fatalf("writes = %v, want the two-row write to land only [%d] after [%d]", got, adenaID, potionID)
	}
	if owner := persistedItemOwner(t, store, second.ObjectID(), potionID); owner != second.ObjectID() {
		t.Fatalf("potion owned by %d after the two-row write, want the receiver %d", owner, second.ObjectID())
	}
	if rows := persistedObjectIDs(t, store, first.ObjectID()); !slices.Equal(rows, []int32{adenaID}) {
		t.Fatalf("first trader's persisted rows = %v, want only the adena [%d]", rows, adenaID)
	}
}
