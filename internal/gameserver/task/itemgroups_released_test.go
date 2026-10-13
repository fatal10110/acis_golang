package task

import (
	"context"
	"slices"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// releasedTrade binds a trade whose giver row was deleted and whose receiver
// stack (row 2) still exists and has a write pending.
func releasedTrade(instances *ItemInstances) *item.Instance {
	receiver := &item.Instance{ObjectID: 2, TemplateID: 1, OwnerID: 200, Count: 40, Location: item.LocationInventory}
	instances.AddOwned(200, receiver)
	instances.Bind([]BoundRow{{ObjectID: 1, OwnerID: 100}, {ObjectID: 2, OwnerID: 200, Inst: receiver}})
	return receiver
}

// TestItemInstancesFailedReleasedFlushReleasesGroup pins that a group whose
// only existing item sits in a released container is released once that
// container's last flush failed, and not before: until the flush runs, it
// still has to carry the dropped rows.
func TestItemInstancesFailedReleasedFlushReleasesGroup(t *testing.T) {
	flusher := &outageFlusher{down: true}
	instances := NewItemInstances(flusher, item.NewTable(nil), nil, nil, zerolog.Nop())
	instances.pendingCap = 0
	receiver := releasedTrade(instances)

	// The cap drops both rows; the owner then leaves, and its container is
	// released, but its last flush has not run yet.
	for range 3 {
		_ = instances.Save(context.Background())
	}
	if instances.ContainsID(2) {
		t.Fatal("row 2 still pending, want it dropped by the cap")
	}
	if got := widenedIDs(instances, 2); !slices.Equal(got, []int32{1}) {
		t.Fatalf("Widen([2]) before the last flush = %v, want the group kept [1]", got)
	}

	if err := instances.FlushReleased(context.Background(), []*item.Instance{receiver}); err == nil {
		t.Fatal("FlushReleased() succeeded, want the outage error")
	}
	if got := batchRowIDs(flusher.last()); !slices.Equal(got, []int32{1, 2}) {
		t.Fatalf("last flush rows = %v, want it to carry the dropped row [1 2]", got)
	}
	if got := widenedIDs(instances, 2); !slices.Equal(got, []int32{1}) {
		t.Fatalf("Widen([2]) right after the failed flush = %v, want the release to wait a Save [1]", got)
	}

	_ = instances.Save(context.Background())
	if n := boundRowCount(instances); n != 0 {
		t.Fatalf("bound rows after the next Save = %d, want 0", n)
	}
}

// TestItemInstancesFailedReleasedFlushKeepsGroupWithLiveRow pins that an
// abandoned row does not release a group another existing item still keeps:
// that item's owner's write still lands both legs.
func TestItemInstancesFailedReleasedFlushKeepsGroupWithLiveRow(t *testing.T) {
	flusher := &outageFlusher{down: true}
	instances := NewItemInstances(flusher, item.NewTable(nil), nil, nil, zerolog.Nop())
	instances.pendingCap = 0
	giver := &item.Instance{ObjectID: 1, TemplateID: 1, OwnerID: 100, Count: 5, Location: item.LocationInventory}
	receiver := &item.Instance{ObjectID: 2, TemplateID: 1, OwnerID: 200, Count: 40, Location: item.LocationInventory}
	instances.Add(giver)
	instances.Add(receiver)
	instances.Bind([]BoundRow{{ObjectID: 1, OwnerID: 100, Inst: giver}, {ObjectID: 2, OwnerID: 200, Inst: receiver}})
	_ = instances.Save(context.Background())

	if err := instances.FlushReleased(context.Background(), []*item.Instance{receiver}); err == nil {
		t.Fatal("FlushReleased() succeeded, want the outage error")
	}
	for range 2 {
		_ = instances.Save(context.Background())
	}
	if got := widenedIDs(instances, 1); !slices.Equal(got, []int32{2}) {
		t.Fatalf("Widen([1]) = %v, want the group kept [2]", got)
	}

	flusher.setDown(false)
	if err := instances.UpdateItems(context.Background(), []*item.Instance{giver}); err != nil {
		t.Fatalf("UpdateItems: %v", err)
	}
	if got := batchRowIDs(flusher.last()); !slices.Equal(got, []int32{1, 2}) {
		t.Fatalf("live owner's write rows = %v, want both legs [1 2]", got)
	}
	if n := boundRowCount(instances); n != 0 {
		t.Fatalf("bound rows after the group landed = %d, want 0", n)
	}
}

// TestItemInstancesFailedReleasedFlushWaitsForPendingWrite pins that an
// abandoned row whose earlier write is still pending keeps its group until
// that write lands or the cap drops it.
func TestItemInstancesFailedReleasedFlushWaitsForPendingWrite(t *testing.T) {
	flusher := &outageFlusher{down: true}
	instances := NewItemInstances(flusher, item.NewTable(nil), nil, nil, zerolog.Nop())
	receiver := releasedTrade(instances)

	if err := instances.FlushReleased(context.Background(), []*item.Instance{receiver}); err == nil {
		t.Fatal("FlushReleased() succeeded, want the outage error")
	}
	_ = instances.Save(context.Background())
	if !instances.ContainsID(2) {
		t.Fatal("row 2 not pending, want the failed tick to keep it")
	}
	if got := widenedIDs(instances, 2); !slices.Equal(got, []int32{1}) {
		t.Fatalf("Widen([2]) while row 2 is pending = %v, want the group kept [1]", got)
	}

	instances.pendingCap = 0
	for range 2 {
		_ = instances.Save(context.Background())
	}
	if n := boundRowCount(instances); n != 0 {
		t.Fatalf("bound rows after the cap dropped the pending write = %d, want 0", n)
	}
}

// TestItemInstancesReleasedFlushLandsGroup pins the landing path: the last
// flush carries the group, settles it and leaves nothing pending.
func TestItemInstancesReleasedFlushLandsGroup(t *testing.T) {
	flusher := &outageFlusher{}
	instances := NewItemInstances(flusher, item.NewTable(nil), nil, nil, zerolog.Nop())
	receiver := releasedTrade(instances)

	if err := instances.FlushReleased(context.Background(), []*item.Instance{receiver}); err != nil {
		t.Fatalf("FlushReleased: %v", err)
	}
	if got := batchRowIDs(flusher.last()); !slices.Equal(got, []int32{1, 2}) {
		t.Fatalf("last flush rows = %v, want both bound rows [1 2]", got)
	}
	if instances.ContainsID(2) {
		t.Fatal("row 2 still pending after its last flush landed")
	}
	if n := boundRowCount(instances); n != 0 {
		t.Fatalf("bound rows after the group landed = %d, want 0", n)
	}
}

// TestItemInstancesFailedReleasedFlushLeavesRetargetedRow pins that a failed
// last flush abandons only rows that still hold its own instances: a row a
// restore has since pointed at a newer instance of the same item is that
// instance's, and keeps the group.
func TestItemInstancesFailedReleasedFlushLeavesRetargetedRow(t *testing.T) {
	flusher := &outageFlusher{down: true}
	instances := NewItemInstances(flusher, item.NewTable(nil), nil, nil, zerolog.Nop())
	instances.pendingCap = 0
	receiver := releasedTrade(instances)
	for range 2 {
		_ = instances.Save(context.Background())
	}
	restored := receiver.Snapshot().Instance()
	instances.retargetGroups(200, map[int32]*item.Instance{2: restored})

	if err := instances.FlushReleased(context.Background(), []*item.Instance{receiver}); err == nil {
		t.Fatal("FlushReleased() succeeded, want the outage error")
	}
	for range 2 {
		_ = instances.Save(context.Background())
	}
	if got := widenedIDs(instances, 2); !slices.Equal(got, []int32{1}) {
		t.Fatalf("Widen([2]) = %v, want the restored row to keep the group [1]", got)
	}
}
