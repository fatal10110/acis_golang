package sql

import (
	"context"
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// BenchmarkItemFlushStore_Flush measures what one lazy-persistence chunk
// costs the database, at the sizes task.ItemInstanceSaveChunkSize is chosen
// from: the per-chunk budget (task.ItemInstanceSaveTimeout) has to cover a
// full chunk with headroom for a degraded server, and the chunk's cost is
// mostly fixed (BEGIN, one multi-row statement per group, COMMIT), so the
// per-item figure shrinks as the chunk grows.
//
// Each batch has the shape a tick produces: every item an upserting save,
// a quarter of them weapons whose augmentation row is cleared, and a tenth
// as many destroyed rows deleted alongside. The rows are re-written on every
// iteration, so the steady state measured is an update of existing rows, the
// common case for a tick.
//
//	go test ./internal/gameserver/data/sql/ -run '^$' -bench BenchmarkItemFlushStore_Flush -benchtime 200x
func BenchmarkItemFlushStore_Flush(b *testing.B) {
	db := sqltest.SharedDB(b)
	store := NewItemFlushStore(db)
	ctx := context.Background()

	for _, size := range []int{1, 10, 100, 500, 1000} {
		batch := benchFlushBatch(size)
		if err := store.Flush(ctx, batch); err != nil {
			b.Fatalf("seed flush of %d items: %v", size, err)
		}
		b.Run(fmt.Sprintf("items=%d", size), func(b *testing.B) {
			for b.Loop() {
				if err := store.Flush(ctx, batch); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*size), "ns/item")
		})
	}
}

func benchFlushBatch(size int) item.FlushBatch {
	const base int32 = 0x20000000
	var batch item.FlushBatch
	for i := range size {
		objectID := base + int32(size)*0x1000 + int32(i)
		batch.Saves = append(batch.Saves, item.InstanceState{
			ObjectID: objectID, TemplateID: 57, OwnerID: 0x10000001, Count: i + 1,
			Location: item.LocationInventory, ManaLeft: -1,
		})
		if i%4 == 0 {
			batch.AugmentationDeletes = append(batch.AugmentationDeletes, objectID)
		}
		if i%10 == 0 {
			batch.Deletes = append(batch.Deletes, objectID+0x800)
		}
	}
	return batch
}
