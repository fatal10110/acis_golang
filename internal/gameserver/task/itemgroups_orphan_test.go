package task

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// outageFlusher records every batch and refuses them while down is set.
type outageFlusher struct {
	mu      sync.Mutex
	down    bool
	batches []item.FlushBatch
}

func (f *outageFlusher) Flush(_ context.Context, batch item.FlushBatch) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.batches = append(f.batches, batch)
	if f.down {
		return errors.New("database unavailable")
	}
	return nil
}

func (f *outageFlusher) setDown(down bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.down = down
}

func (f *outageFlusher) last() item.FlushBatch {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.batches[len(f.batches)-1]
}

func boundRowCount(instances *ItemInstances) int {
	instances.groupsMu.Lock()
	defer instances.groupsMu.Unlock()
	return len(instances.groups)
}

func widenedIDs(instances *ItemInstances, ids ...int32) []int32 {
	rows, _ := instances.Widen(ids)
	out := make([]int32, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ObjectID)
	}
	slices.Sort(out)
	return out
}

// usedUpTrade binds a trade whose giver row was deleted and whose receiver
// stack (row 2) was then used up, with the receiver's write pending.
func usedUpTrade(instances *ItemInstances) *item.Instance {
	receiver := &item.Instance{ObjectID: 2, TemplateID: 1, OwnerID: 200, Count: 0, Location: item.LocationInventory}
	instances.AddOwned(200, receiver)
	instances.Bind([]BoundRow{{ObjectID: 1, OwnerID: 100}, {ObjectID: 2, OwnerID: 200, Inst: receiver}})
	return receiver
}

// TestItemInstancesCapDropReleasesGroupWithNoRowLeft pins that a bound group
// whose rows the pending cap dropped, and none of whose items still exists, is
// released by the next Save instead of being held for the life of the process.
func TestItemInstancesCapDropReleasesGroupWithNoRowLeft(t *testing.T) {
	flusher := &outageFlusher{down: true}
	instances := NewItemInstances(flusher, item.NewTable(nil), nil, nil, zerolog.Nop())
	instances.pendingCap = 0
	usedUpTrade(instances)

	if err := instances.Save(context.Background()); err == nil {
		t.Fatal("Save() succeeded, want the outage error")
	}
	if instances.ContainsID(2) {
		t.Fatal("row 2 still pending, want it dropped by the cap")
	}
	// The release waits a Save, so a write still on its way in keeps the
	// group (see TestItemInstancesLateWriteOfDroppedRowKeepsGroup).
	if got := widenedIDs(instances, 2); !slices.Equal(got, []int32{1}) {
		t.Fatalf("Widen([2]) right after the drop = %v, want the group still bound [1]", got)
	}

	if err := instances.Save(context.Background()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if n := boundRowCount(instances); n != 0 {
		t.Fatalf("bound rows after the next Save = %d, want 0", n)
	}
	instances.groupsMu.Lock()
	orphans := len(instances.orphans)
	instances.groupsMu.Unlock()
	if orphans != 0 {
		t.Fatalf("orphan marks after the release = %d, want 0", orphans)
	}
}

// TestItemInstancesLateWriteOfDroppedRowKeepsGroup pins why the release waits
// a Save: a used-up row's write that reaches pending after its earlier write
// was dropped must still carry the rest of the group.
func TestItemInstancesLateWriteOfDroppedRowKeepsGroup(t *testing.T) {
	flusher := &outageFlusher{down: true}
	instances := NewItemInstances(flusher, item.NewTable(nil), nil, nil, zerolog.Nop())
	instances.pendingCap = 0
	receiver := usedUpTrade(instances)
	if err := instances.Save(context.Background()); err == nil {
		t.Fatal("Save() succeeded, want the outage error")
	}

	instances.AddOwned(200, receiver)
	flusher.setDown(false)
	if err := instances.Save(context.Background()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := batchRowIDs(flusher.last()); !slices.Equal(got, []int32{1, 2}) {
		t.Fatalf("late write rows = %v, want both bound rows [1 2]", got)
	}
	if n := boundRowCount(instances); n != 0 {
		t.Fatalf("bound rows after the group landed = %d, want 0", n)
	}
}

// TestItemInstancesCapDropOfOneRowSurvivorCarriesIt pins that a group whose
// rows the cap dropped only in part keeps the dropped row: the surviving
// member's write still lands it.
func TestItemInstancesCapDropOfOneRowSurvivorCarriesIt(t *testing.T) {
	flusher := &outageFlusher{down: true}
	instances := NewItemInstances(flusher, item.NewTable(nil), nil, nil, zerolog.Nop())
	instances.pendingCap = 1
	receiver := &item.Instance{ObjectID: 2, TemplateID: 1, OwnerID: 200, Count: 40, Location: item.LocationInventory}
	giver := &item.Instance{ObjectID: 1, TemplateID: 1, OwnerID: 100, Count: 0, Location: item.LocationInventory}
	// The receiver waits longest, so its failed write keeps the one pending
	// place and the giver's is dropped.
	instances.Add(receiver)
	instances.Add(giver)
	instances.Bind([]BoundRow{{ObjectID: 1, OwnerID: 100, Inst: giver}, {ObjectID: 2, OwnerID: 200, Inst: receiver}})

	if err := instances.Save(context.Background()); err == nil {
		t.Fatal("Save() succeeded, want the outage error")
	}
	if !instances.ContainsID(2) || instances.ContainsID(1) {
		t.Fatalf("pending after the outage: receiver %v giver %v, want only the receiver", instances.ContainsID(2), instances.ContainsID(1))
	}

	flusher.setDown(false)
	if err := instances.Save(context.Background()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := batchRowIDs(flusher.last()); !slices.Equal(got, []int32{1, 2}) {
		t.Fatalf("surviving write rows = %v, want it to carry the dropped row [1 2]", got)
	}
	if n := boundRowCount(instances); n != 0 {
		t.Fatalf("bound rows after the group landed = %d, want 0", n)
	}
}

// TestItemInstancesCapDropKeepsGroupOfExistingItem pins that a group whose
// every write the cap dropped is kept while one of its items still exists:
// the owner's later write of it, such as the container's last flush at
// logout, must still carry the dropped rows.
func TestItemInstancesCapDropKeepsGroupOfExistingItem(t *testing.T) {
	flusher := &outageFlusher{down: true}
	instances := NewItemInstances(flusher, item.NewTable(nil), nil, nil, zerolog.Nop())
	instances.pendingCap = 0
	receiver := &item.Instance{ObjectID: 2, TemplateID: 1, OwnerID: 200, Count: 40, Location: item.LocationInventory}
	instances.Add(receiver)
	instances.Bind([]BoundRow{{ObjectID: 1, OwnerID: 100}, {ObjectID: 2, OwnerID: 200, Inst: receiver}})

	for range 2 {
		_ = instances.Save(context.Background())
	}
	if instances.ContainsID(2) {
		t.Fatal("row 2 still pending, want it dropped by the cap")
	}
	if got := widenedIDs(instances, 2); !slices.Equal(got, []int32{1}) {
		t.Fatalf("Widen([2]) = %v, want the group kept [1]", got)
	}

	flusher.setDown(false)
	if err := instances.UpdateItems(context.Background(), []*item.Instance{receiver}); err != nil {
		t.Fatalf("UpdateItems: %v", err)
	}
	if got := batchRowIDs(flusher.last()); !slices.Equal(got, []int32{1, 2}) {
		t.Fatalf("owner's write rows = %v, want it to carry the dropped row [1 2]", got)
	}
}

// TestItemInstancesCapDropReleaseLeavesMergedGroup pins that a group bound
// again after it was marked, now holding an item that still exists, is not
// released.
func TestItemInstancesCapDropReleaseLeavesMergedGroup(t *testing.T) {
	flusher := &outageFlusher{down: true}
	instances := NewItemInstances(flusher, item.NewTable(nil), nil, nil, zerolog.Nop())
	instances.pendingCap = 0
	receiver := usedUpTrade(instances)
	if err := instances.Save(context.Background()); err == nil {
		t.Fatal("Save() succeeded, want the outage error")
	}

	live := &item.Instance{ObjectID: 3, TemplateID: 1, OwnerID: 200, Count: 5, Location: item.LocationInventory}
	instances.Bind([]BoundRow{{ObjectID: 2, OwnerID: 200, Inst: receiver}, {ObjectID: 3, OwnerID: 200, Inst: live}})
	if err := instances.Save(context.Background()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := widenedIDs(instances, 3); !slices.Equal(got, []int32{1, 2}) {
		t.Fatalf("Widen([3]) = %v, want the merged group kept [1 2]", got)
	}
}
