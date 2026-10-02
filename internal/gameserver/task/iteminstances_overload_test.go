package task

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
)

// outcomeFlusher records the ids of every flush's saves and runs onFlush, if
// set, to decide the flush's outcome.
type outcomeFlusher struct {
	mu      sync.Mutex
	calls   [][]int32
	onFlush func(ctx context.Context, call int) error
}

func (f *outcomeFlusher) Flush(ctx context.Context, batch item.FlushBatch) error {
	f.mu.Lock()
	call := len(f.calls)
	f.calls = append(f.calls, savedIDs(batch.Saves))
	hook := f.onFlush
	f.mu.Unlock()
	if hook == nil {
		return nil
	}
	return hook(ctx, call)
}

func (f *outcomeFlusher) flushed() [][]int32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func ownedItem(objectID, ownerID int32) *item.Instance {
	return &item.Instance{ObjectID: objectID, TemplateID: 1, OwnerID: ownerID, Count: 1, Location: item.LocationInventory}
}

func idRange(from, to int32) []int32 {
	var ids []int32
	for id := from; id <= to; id++ {
		ids = append(ids, id)
	}
	return ids
}

// TestItemInstancesSaveDoesNotStarveTheRowsAnOverloadedTickCutOff runs two
// overloaded ticks — each has room for one chunk before its ctx ends. The
// rows the first tick never reached must be the second tick's first chunk,
// even though the rows it did write changed again since and have lower
// object ids. An order by object id would write the same low ids every tick
// and never reach the rest.
func TestItemInstancesSaveDoesNotStarveTheRowsAnOverloadedTickCutOff(t *testing.T) {
	flusher := &outcomeFlusher{}
	instances := NewItemInstances(flusher, item.NewTable(nil), nil, nil, zerolog.Nop())
	items := addSequentialPending(instances, 2*ItemInstanceSaveChunkSize)

	overloadedSave := func() {
		t.Helper()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		flusher.onFlush = func(context.Context, int) error { cancel(); return nil }
		if err := instances.Save(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("Save() error = %v, want context.Canceled", err)
		}
	}

	overloadedSave()
	// The rows the first tick wrote change again, and newer items arrive
	// with the lowest ids of all.
	for _, inst := range items[:ItemInstanceSaveChunkSize] {
		instances.Add(inst)
	}
	overloadedSave()

	calls := flusher.flushed()
	if len(calls) != 2 {
		t.Fatalf("Flush called %d times, want one chunk per tick", len(calls))
	}
	if want := idRange(1, ItemInstanceSaveChunkSize); !slices.Equal(calls[0], want) {
		t.Fatalf("first tick wrote %v, want %v", calls[0], want)
	}
	if want := idRange(ItemInstanceSaveChunkSize+1, 2*ItemInstanceSaveChunkSize); !slices.Equal(calls[1], want) {
		t.Fatalf("second tick wrote %v, want the rows the first tick never reached %v", calls[1], want)
	}
	if !instances.Contains(items[0]) {
		t.Fatal("the re-changed rows must still be pending for the next tick")
	}
}

// TestItemInstancesSaveDispatchesOwnersLongestWaitingFirst pins the same
// order across owners: a tick dispatches the owner whose row has waited
// longest first, whatever its id, so a ctx that runs out leaves the newest
// owner's rows for the next tick rather than always the highest owner id's.
func TestItemInstancesSaveDispatchesOwnersLongestWaitingFirst(t *testing.T) {
	flusher := &outcomeFlusher{}
	instances := NewItemInstances(flusher, item.NewTable(nil), nil, nil, zerolog.Nop())
	older := ownedItem(500, 9)
	newer := ownedItem(10, 1)
	instances.Add(older)
	instances.Add(newer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	flusher.onFlush = func(context.Context, int) error { cancel(); return nil }
	if err := instances.Save(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Save() error = %v, want context.Canceled", err)
	}

	if calls := flusher.flushed(); len(calls) != 1 || !slices.Equal(calls[0], []int32{500}) {
		t.Fatalf("flushes = %v, want only the longest-waiting owner's row [500]", calls)
	}
	if !instances.Contains(newer) || instances.Contains(older) {
		t.Fatal("the newer owner's row must be the one left pending")
	}
}

// TestItemInstancesFailedWriteKeepsItsPlace covers a row that changes again
// while its write is out and failing. The row has been stale since its first
// change, so the next tick must still put it ahead of a row that went stale
// during that write, rather than behind it as a fresh change.
func TestItemInstancesFailedWriteKeepsItsPlace(t *testing.T) {
	flusher := &outcomeFlusher{}
	instances := NewItemInstances(flusher, item.NewTable(nil), nil, nil, zerolog.Nop())
	retried := ownedItem(50, 2)
	instances.Add(retried)
	arrived := ownedItem(10, 1)

	flusher.onFlush = func(context.Context, int) error {
		instances.Add(arrived)
		instances.Add(retried)
		return errors.New("database unavailable")
	}
	if err := instances.Save(context.Background()); err == nil {
		t.Fatal("Save() succeeded, want the flush error")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	flusher.onFlush = func(context.Context, int) error { cancel(); return nil }
	if err := instances.Save(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Save() error = %v, want context.Canceled", err)
	}
	calls := flusher.flushed()
	if len(calls) != 2 || !slices.Equal(calls[1], []int32{50}) {
		t.Fatalf("flushes = %v, want the retried row [50] first on the second tick", calls)
	}
}

// TestItemInstancesPendingCapDropsFailedWritesPastIt fills the pending set
// past its cap with writes that all fail. The set must stop at the cap,
// keeping the longest-waiting rows and dropping the newest, while a change
// made after the failure is still accepted.
func TestItemInstancesPendingCapDropsFailedWritesPastIt(t *testing.T) {
	flusher := &outcomeFlusher{onFlush: func(context.Context, int) error { return errors.New("database unavailable") }}
	instances := NewItemInstances(flusher, item.NewTable(nil), nil, nil, zerolog.Nop())
	instances.pendingCap = 3
	items := addSequentialPending(instances, 5)

	if err := instances.Save(context.Background()); err == nil {
		t.Fatal("Save() succeeded, want the flush error")
	}
	for idx, inst := range items {
		if got, want := instances.Contains(inst), idx < 3; got != want {
			t.Fatalf("item %d pending = %v, want %v: the cap keeps the three longest-waiting rows", inst.ObjectID, got, want)
		}
	}

	fresh := ownedItem(99, 0)
	instances.Add(fresh)
	if !instances.Contains(fresh) {
		t.Fatal("a new change must be accepted even with the pending set at its cap")
	}
}

// TestItemInstancesPendingCapIgnoresRowsAlreadyPending pins that the cap
// counts rows, not writes: a failed write of a row that changed again during
// the flush folds into the pending entry and is never dropped for the cap.
func TestItemInstancesPendingCapIgnoresRowsAlreadyPending(t *testing.T) {
	flusher := &outcomeFlusher{}
	instances := NewItemInstances(flusher, item.NewTable(nil), nil, nil, zerolog.Nop())
	instances.pendingCap = 1
	inst := ownedItem(7, 1)
	instances.Add(inst)
	flusher.onFlush = func(context.Context, int) error {
		instances.Add(inst)
		return errors.New("database unavailable")
	}
	if err := instances.Save(context.Background()); err == nil {
		t.Fatal("Save() succeeded, want the flush error")
	}
	if !instances.Contains(inst) {
		t.Fatal("the row must stay pending")
	}
}

// TestItemInstancesStopCutsALongTickShort stops the item ticker while its
// Save is stuck in a database call that only ends with its ctx. The stop
// must cancel the tick rather than wait out ItemInstanceTickBudget, which
// is what lets that budget exceed the shutdown's own steps, and the cut-off
// row must stay pending for the shutdown drain.
func TestItemInstancesStopCutsALongTickShort(t *testing.T) {
	const stopWait = 2 * time.Second
	if ItemInstanceTickBudget <= stopWait {
		t.Fatalf("ItemInstanceTickBudget = %s is no longer than the stop this test allows; the test proves nothing", ItemInstanceTickBudget)
	}
	worker := persist.New(zerolog.Nop())
	defer worker.Close(context.Background())

	entered := make(chan struct{})
	var once sync.Once
	flusher := &outcomeFlusher{onFlush: func(ctx context.Context, _ int) error {
		once.Do(func() { close(entered) })
		<-ctx.Done()
		return ctx.Err()
	}}
	instances := NewItemInstances(flusher, item.NewTable(nil), worker, nil, zerolog.Nop())
	inst := ownedItem(1, 7)
	instances.Add(inst)

	ticker := instances.start(5*time.Millisecond, zerolog.Nop())
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		ticker.StopAndWait()
		t.Fatal("the tick never reached the database")
	}

	stopped := make(chan struct{})
	go func() {
		ticker.StopAndWait()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(stopWait):
		t.Fatalf("stopping the ticker waited on the in-flight save past %s", stopWait)
	}

	if err := worker.Flush(context.Background(), inst.OwnerID); err != nil {
		t.Fatal(err)
	}
	if !instances.Contains(inst) {
		t.Fatal("the row the stop cut short must stay pending for the shutdown drain")
	}
}
