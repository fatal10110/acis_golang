package network

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// recordingFlusher records what each flush batch did to one row, so a test can
// assert the order the row's writes landed in rather than only the state they
// left behind — a write that lands on top of a delete leaves the same row as
// one that was never deleted.
type recordingFlusher struct {
	inner   task.ItemFlusher
	watch   int32
	mu      sync.Mutex
	applied []string
}

func (f *recordingFlusher) Flush(ctx context.Context, batch item.FlushBatch) error {
	if err := f.inner.Flush(ctx, batch); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, st := range batch.Saves {
		if st.ObjectID == f.watch {
			f.applied = append(f.applied, fmt.Sprintf("save(%d)", st.Count))
		}
	}
	for _, id := range batch.Deletes {
		if id == f.watch {
			f.applied = append(f.applied, "delete")
		}
	}
	return nil
}

// recordingItemStore records the single-row writes a handler makes, into the
// same log as the flusher's batches, so the log is every write of the row in
// the order it landed.
type recordingItemStore struct {
	itemStore
	log   *recordingFlusher
	watch int32
}

func (s recordingItemStore) SaveState(ctx context.Context, st item.InstanceState) error {
	if err := s.itemStore.SaveState(ctx, st); err != nil {
		return err
	}
	if st.ObjectID == s.watch {
		s.log.record(fmt.Sprintf("save(%d)", st.Count))
	}
	return nil
}

func (s recordingItemStore) Delete(ctx context.Context, objectID int32) error {
	if err := s.itemStore.Delete(ctx, objectID); err != nil {
		return err
	}
	if objectID == s.watch {
		s.log.record("delete")
	}
	return nil
}

func (f *recordingFlusher) record(op string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applied = append(f.applied, op)
}

func (f *recordingFlusher) ops() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.applied...)
}

// TestTickWriteCannotBeOvertakenByAnEarlierHandlerWrite covers the pairing the
// ordering rests on: a write's place in its row's order and the state it will
// land have to describe the same moment. The lazy-persistence tick reads its
// rows after waiting for them, so a flush that took its place first could hold
// an older place while carrying a newer state — and a single-row write
// produced in between, holding the later place, would then land on top of it.
//
// The interleaving is the one that makes the two disagree: the tick waits on a
// row another lane is holding while the owner trades part of the stack away
// and then consumes the rest.
func TestTickWriteCannotBeOvertakenByAnEarlierHandlerWrite(t *testing.T) {
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

	const objectID int32 = 5300
	flusher := &recordingFlusher{inner: gamesql.NewItemFlushStore(sqltest.SharedDB(t)), watch: objectID}
	instances := task.NewItemInstances(flusher, link.itemTemplates, worker, order)
	link.itemInstances = instances
	link.items = recordingItemStore{itemStore: link.items, log: flusher, watch: objectID}

	inv := first.Inventory()
	// The fixture's inventory carries no persister of its own; the stack is
	// bound explicitly. Container-to-persister scheduling is covered in
	// itemcontainer's tests.
	persister := &ownerItemPersister{instances: instances, ownerID: inv.OwnerID()}
	stack := inv.AddNew(item.AdenaID, 101, objectID)
	stack.BindPersister(persister)
	persister.Persist(stack)
	inv.DrainUpdates()

	// Another lane holds the row, so the tick's flush waits for it with its
	// state already read. Owner 4 maps to a different lane than owner 1, so
	// the hold contends the row and not the lane the tick runs on.
	holder := order.Reserve(objectID)
	held, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releaseRow := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseRow)
	worker.Enqueue(4, func() {
		holder.Run(func([]int32) {
			close(held)
			<-release
		})
	})
	<-held

	saved := make(chan error, 1)
	go func() { saved <- instances.Save(context.Background()) }()
	waitForTickToReserve(t, order, objectID)

	// One unit goes to the other trader: a single-row write, produced after
	// the tick read its state, carrying the remaining 100.
	res, ok, err := link.inventory.TransferItem(inv, second.Inventory(), objectID, 1)
	if err != nil || !ok {
		t.Fatalf("TransferItem() = ok %v, err %v", ok, err)
	}
	link.applyPersistActions(res.Persist)

	// The rest is consumed, which only registers the row for the next tick.
	if inv.DestroyItem(stack, 100) == nil {
		t.Fatal("DestroyItem() returned nil")
	}

	releaseRow()
	if err := <-saved; err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := worker.Flush(context.Background()); err != nil {
		t.Fatalf("flush lanes: %v", err)
	}

	ops := flusher.ops()
	for i, op := range ops {
		if op == "delete" && i != len(ops)-1 {
			t.Fatalf("a write landed on top of the row's delete: %v", ops)
		}
	}
	if len(ops) == 0 {
		t.Fatal("no writes were recorded: the interleaving did not happen")
	}

	// The consumed stack is still registered, so the next tick takes it away.
	if err := instances.Save(context.Background()); err != nil {
		t.Fatalf("second Save() error = %v", err)
	}
	if err := worker.Flush(context.Background()); err != nil {
		t.Fatalf("flush lanes after the second tick: %v", err)
	}
	for _, inst := range mustListItems(t, store, first.ObjectID()) {
		if inst.ObjectID == objectID {
			t.Fatalf("consumed stack still persisted: count %d, writes %v", inst.Count, flusher.ops())
		}
	}
}

// waitForTickToReserve blocks until the flush has taken its place for
// objectID, which is what puts the handler write after it.
func waitForTickToReserve(t *testing.T, order *persist.Order, objectID int32) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if order.Reserved(objectID) > 1 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the flush never took its place for the row")
}

func mustListItems(t *testing.T, store *gamesql.ItemStore, ownerID int32) []*item.Instance {
	t.Helper()
	instances, err := store.ListByOwner(context.Background(), ownerID)
	if err != nil {
		t.Fatalf("list items for owner %d: %v", ownerID, err)
	}
	return instances
}
