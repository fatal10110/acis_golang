package network

import (
	"context"
	"testing"

	"github.com/rs/zerolog"

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
