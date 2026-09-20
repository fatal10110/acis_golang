package network

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"sync/atomic"

	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
)

// TestQueuedItemWriteCannotRevertAnOwnershipTransfer pins what a queued
// item-row write may land once the item has changed hands. Writes are queued
// on the lane of the row's owner when the action is produced, so an item that
// moves from one player to another has its two writes on two lanes, and the
// giver's lane can drain last. A write that carried a state frozen when it was
// queued would then overwrite the row with the previous owner, handing the
// item back to the giver — the row has to end up with whoever holds the item.
//
// The giver's lane is held behind a barrier rather than timed, so the
// interleaving is fixed.
func TestQueuedItemWriteCannotRevertAnOwnershipTransfer(t *testing.T) {
	link, store, _, _, first, second := newDirectTradeFixture(t)
	worker := persist.New(zerolog.Nop())
	t.Cleanup(func() {
		if err := worker.Close(context.Background()); err != nil {
			t.Errorf("close persistence worker: %v", err)
		}
	})
	link.persist = worker

	const objectID int32 = 5000
	const count = 100
	adena := first.Inventory().AddNew(item.AdenaID, count, objectID)
	first.Inventory().DrainUpdates()

	release := make(chan struct{})
	worker.Enqueue(first.ObjectID(), func() { <-release })
	link.applyPersistActions([]invops.Persist{invops.Save(adena)})

	res, ok, err := link.inventory.TransferItem(first.Inventory(), second.Inventory(), objectID, count)
	if err != nil || !ok {
		t.Fatalf("TransferItem() = ok %v, err %v", ok, err)
	}
	link.applyPersistActions(res.Persist)

	// The receiver's lane lands its write first; the giver's runs after.
	if err := worker.Flush(context.Background(), second.ObjectID()); err != nil {
		t.Fatalf("flush receiver lane: %v", err)
	}
	close(release)
	if err := worker.Flush(context.Background(), first.ObjectID()); err != nil {
		t.Fatalf("flush giver lane: %v", err)
	}

	if owner := persistedItemOwner(t, store, second.ObjectID(), objectID); owner != second.ObjectID() {
		t.Fatalf("item %d is owned by %d after the transfer, want the receiver %d", objectID, owner, second.ObjectID())
	}
	if rows := persistedObjectIDs(t, store, first.ObjectID()); len(rows) != 0 {
		t.Fatalf("giver still owns persisted rows %v after handing the stack over", rows)
	}
}

func persistedItemOwner(t *testing.T, store interface {
	ListByOwner(context.Context, int32) ([]*item.Instance, error)
}, ownerID, objectID int32) int32 {
	t.Helper()
	instances, err := store.ListByOwner(context.Background(), ownerID)
	if err != nil {
		t.Fatalf("list items for owner %d: %v", ownerID, err)
	}
	for _, inst := range instances {
		if inst.ObjectID == objectID {
			return inst.OwnerID
		}
	}
	t.Fatalf("no persisted row for object %d under owner %d", objectID, ownerID)
	return 0
}

func persistedObjectIDs(t *testing.T, store interface {
	ListByOwner(context.Context, int32) ([]*item.Instance, error)
}, ownerID int32) []int32 {
	t.Helper()
	instances, err := store.ListByOwner(context.Background(), ownerID)
	if err != nil {
		t.Fatalf("list items for owner %d: %v", ownerID, err)
	}
	out := make([]int32, 0, len(instances))
	for _, inst := range instances {
		out = append(out, inst.ObjectID)
	}
	return out
}

// blockingItemStore holds one object's state write inside the store, after it
// has been handed the state to write. That is the gap a snapshot taken in the
// job does not cover: the write has already read what it will land and is
// waiting on the database while other lanes commit.
type blockingItemStore struct {
	itemStore
	objectID int32
	entered  chan struct{}
	release  chan struct{}
	held     atomic.Bool
}

// SaveState holds only the first write of the watched row, so a later write
// of the same row is free to reach the database while that one waits.
func (s *blockingItemStore) SaveState(ctx context.Context, st item.InstanceState) error {
	if st.ObjectID == s.objectID && s.held.CompareAndSwap(false, true) {
		close(s.entered)
		<-s.release
	}
	return s.itemStore.SaveState(ctx, st)
}

// TestInFlightItemWriteCannotRevertAnOwnershipTransfer is the same invariant
// as the test above for a write that has already started. The giver's write
// is blocked inside the store, holding the state it read, while the item is
// transferred and the receiver's write is queued behind it. Whichever order
// the two lanes take, the row has to end up with the receiver: reading the
// state later only moves the gap, so the rows themselves have to be
// serialized (persist.Order).
func TestInFlightItemWriteCannotRevertAnOwnershipTransfer(t *testing.T) {
	link, store, _, _, first, second := newDirectTradeFixture(t)
	worker := persist.New(zerolog.Nop())
	t.Cleanup(func() {
		if err := worker.Close(context.Background()); err != nil {
			t.Errorf("close persistence worker: %v", err)
		}
	})
	link.persist = worker

	const objectID int32 = 5100
	const count = 100
	blocking := &blockingItemStore{
		itemStore: link.items,
		objectID:  objectID,
		entered:   make(chan struct{}),
		release:   make(chan struct{}),
	}
	link.items = blocking

	adena := first.Inventory().AddNew(item.AdenaID, count, objectID)
	first.Inventory().DrainUpdates()
	link.applyPersistActions([]invops.Persist{invops.Save(adena)})

	select {
	case <-blocking.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the giver's write never reached the store")
	}

	res, ok, err := link.inventory.TransferItem(first.Inventory(), second.Inventory(), objectID, count)
	if err != nil || !ok {
		close(blocking.release)
		t.Fatalf("TransferItem() = ok %v, err %v", ok, err)
	}
	link.applyPersistActions(res.Persist)

	// Give the receiver's lane every chance to land first while the giver's
	// write is still inside the store.
	flushed := make(chan error, 1)
	go func() { flushed <- worker.Flush(context.Background(), second.ObjectID()) }()
	select {
	case err := <-flushed:
		if err != nil {
			close(blocking.release)
			t.Fatalf("flush receiver lane: %v", err)
		}
	case <-time.After(300 * time.Millisecond):
	}

	close(blocking.release)
	if err := worker.Flush(context.Background(), first.ObjectID(), second.ObjectID()); err != nil {
		t.Fatalf("flush lanes: %v", err)
	}

	if owner := persistedItemOwner(t, store, second.ObjectID(), objectID); owner != second.ObjectID() {
		t.Fatalf("item %d is owned by %d after the transfer, want the receiver %d", objectID, owner, second.ObjectID())
	}
}

// TestQueuedItemWriteCannotDeleteARepickedRow covers the other way a queued
// write can reach a row that is no longer its own: a dropped item leaves its
// inventory instance behind at owner 0 and location VOID, and a pickup builds
// a *new* instance carrying the same object id (inventory.PickupGround). The
// giver's queued write still points at the replaced instance, so neither its
// state nor its identity describes the row any more — only the row's write
// order can tell that the new holder's write is the later one.
func TestQueuedItemWriteCannotDeleteARepickedRow(t *testing.T) {
	link, store, _, _, first, second := newDirectTradeFixture(t)
	worker := persist.New(zerolog.Nop())
	t.Cleanup(func() {
		if err := worker.Close(context.Background()); err != nil {
			t.Errorf("close persistence worker: %v", err)
		}
	})
	link.persist = worker

	const objectID int32 = 5200
	const count = 100
	dropped := first.Inventory().AddNew(item.AdenaID, count, objectID)
	first.Inventory().DrainUpdates()

	// The giver's write waits on a held lane, still holding the instance it
	// was produced with.
	release := make(chan struct{})
	worker.Enqueue(first.ObjectID(), func() { <-release })
	link.applyPersistActions([]invops.Persist{invops.Save(dropped)})

	if !first.Inventory().Remove(dropped, true) {
		t.Fatal("drop did not remove the item from the giver's inventory")
	}
	tmpl, ok := first.Inventory().Templates().Get(item.AdenaID)
	if !ok {
		t.Fatal("missing adena template")
	}
	picked, failure := link.inventory.PickupGround(second.Inventory(), dropped, tmpl, second.ObjectID())
	if failure != invops.PickupOK {
		close(release)
		t.Fatalf("PickupGround() failure = %v", failure)
	}
	link.applyPersistActions(picked.Persist)
	if err := worker.Flush(context.Background(), second.ObjectID()); err != nil {
		close(release)
		t.Fatalf("flush receiver lane: %v", err)
	}
	if owner := persistedItemOwner(t, store, second.ObjectID(), objectID); owner != second.ObjectID() {
		close(release)
		t.Fatalf("pickup did not persist the row under the new holder: owner %d", owner)
	}

	close(release)
	if err := worker.Flush(context.Background(), first.ObjectID(), second.ObjectID()); err != nil {
		t.Fatalf("flush lanes: %v", err)
	}

	if owner := persistedItemOwner(t, store, second.ObjectID(), objectID); owner != second.ObjectID() {
		t.Fatalf("item %d is owned by %d after the pickup, want the new holder %d", objectID, owner, second.ObjectID())
	}
}
