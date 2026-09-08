package task

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// chunkTrackingFlusher records every Flush call's batch (unlike
// itemFlusherStub, which only keeps the most recent one), so a multi-chunk
// Save's per-chunk composition and call count can be asserted. The first
// call sleeps for delay before returning, letting a test race Save's
// between-chunk ctx.Err() check against an outer deadline deterministically.
type chunkTrackingFlusher struct {
	mu      sync.Mutex
	batches []item.FlushBatch
	delay   time.Duration
}

func (f *chunkTrackingFlusher) Flush(_ context.Context, batch item.FlushBatch) error {
	f.mu.Lock()
	first := len(f.batches) == 0
	f.batches = append(f.batches, batch)
	f.mu.Unlock()
	if first && f.delay > 0 {
		time.Sleep(f.delay)
	}
	return nil
}

func (f *chunkTrackingFlusher) calls() []item.FlushBatch {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]item.FlushBatch(nil), f.batches...)
}

// TestItemInstancesSaveDeadlineStopsUnattemptedChunksButKeepsEarlierCommits
// pins the mixed case a #2295 review flagged as untested: a Save whose
// outer ctx expires partway through a multi-chunk flush must keep the
// chunks that already committed out of pending, while every chunk it never
// got to attempt (the ctx.Err() != nil branch) goes back in. The first
// chunk's Flush call is made to outlast a short outer deadline, so the
// second chunk is provably never attempted rather than merely failing fast.
func TestItemInstancesSaveDeadlineStopsUnattemptedChunksButKeepsEarlierCommits(t *testing.T) {
	flusher := &chunkTrackingFlusher{delay: 50 * time.Millisecond}
	instances := NewItemInstances(flusher, item.NewTable(nil))

	const total = 2 * ItemInstanceSaveChunkSize
	items := make([]*item.Instance, total)
	for idx := range items {
		inst := &item.Instance{ObjectID: int32(idx + 1), TemplateID: 1, Count: 1, Location: item.LocationInventory}
		items[idx] = inst
		instances.Add(inst)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := instances.Save(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Save() error = %v, want context.DeadlineExceeded", err)
	}

	calls := flusher.calls()
	if len(calls) != 1 {
		t.Fatalf("Flush called %d times, want exactly 1: the second chunk must never be attempted once ctx has expired", len(calls))
	}
	if got := len(calls[0].Saves); got != ItemInstanceSaveChunkSize {
		t.Fatalf("first chunk's batch saved %d items, want %d", got, ItemInstanceSaveChunkSize)
	}

	if instances.Contains(items[0]) {
		t.Fatal("an item from the committed first chunk must not be pending")
	}
	if !instances.Contains(items[total-1]) {
		t.Fatal("an item from the never-attempted second chunk must stay pending for retry")
	}
}
