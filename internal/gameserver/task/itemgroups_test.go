package task

import (
	"context"
	"slices"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
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
// widened as a bare delete, and only a write that carried every row settles
// the group.
func TestItemInstancesBindMergesAndLandsWhole(t *testing.T) {
	instances := NewItemInstances(nil, item.NewTable(nil), nil, nil, zerolog.Nop())
	one := &item.Instance{ObjectID: 1, OwnerID: 100}
	two := &item.Instance{ObjectID: 2, OwnerID: 200}
	instances.Bind([]BoundRow{{ObjectID: 1, OwnerID: 100, Inst: one}, {ObjectID: 2, OwnerID: 200, Inst: two}})
	instances.Bind([]BoundRow{{ObjectID: 2, OwnerID: 200, Inst: two}, {ObjectID: 3, OwnerID: 200}})

	widened := instances.Widen([]int32{1})
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

	instances.Landed([]int32{1, 2})
	if got := instances.Widen([]int32{1}); len(got) != 2 {
		t.Fatalf("Widen after a partial landing = %+v, want the group intact", got)
	}
	instances.Landed([]int32{1, 2, 3})
	if got := instances.Widen([]int32{1, 2, 3}); len(got) != 0 {
		t.Fatalf("Widen after the whole group landed = %+v, want nothing", got)
	}
}
