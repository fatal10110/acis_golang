package task

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
)

// chunkTrackingFlusher records every Flush call's batch and the deadline its
// ctx carried (unlike itemFlusherStub, which only keeps the most recent
// batch and ignores ctx entirely), so a multi-chunk Save's per-chunk
// composition and per-chunk timeout can both be asserted. If cancel is set,
// the first call invokes it after recording, letting a test deterministically
// race Save's between-chunk ctx.Err() check against a canceled ctx without
// any wall-clock sleep.
type chunkTrackingFlusher struct {
	mu        sync.Mutex
	batches   []item.FlushBatch
	deadlines []flushDeadline
	cancel    context.CancelFunc
}

type flushDeadline struct {
	at time.Time
	ok bool
}

func (f *chunkTrackingFlusher) Flush(ctx context.Context, batch item.FlushBatch) error {
	f.mu.Lock()
	first := len(f.batches) == 0
	f.batches = append(f.batches, batch)
	at, ok := ctx.Deadline()
	f.deadlines = append(f.deadlines, flushDeadline{at: at, ok: ok})
	f.mu.Unlock()
	if first && f.cancel != nil {
		f.cancel()
	}
	return nil
}

func (f *chunkTrackingFlusher) calls() []item.FlushBatch {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]item.FlushBatch(nil), f.batches...)
}

func (f *chunkTrackingFlusher) calledDeadlines() []flushDeadline {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]flushDeadline(nil), f.deadlines...)
}

func addSequentialPending(instances *ItemInstances, n int) []*item.Instance {
	items := make([]*item.Instance, n)
	for idx := range items {
		inst := &item.Instance{ObjectID: int32(idx + 1), TemplateID: 1, Count: 1, Location: item.LocationInventory}
		items[idx] = inst
		instances.Add(inst)
	}
	return items
}

// TestItemInstancesSaveDeadlineStopsUnattemptedChunksButKeepsEarlierCommits
// pins the mixed case a #2295 review flagged as untested: a Save whose ctx
// is canceled partway through a multi-chunk flush must keep the chunks that
// already committed out of pending, while every chunk it never got to
// attempt (the ctx.Err() != nil branch) goes back in. The first chunk's
// Flush call cancels ctx itself right after being recorded, so the second
// chunk is deterministically never attempted rather than merely racing a
// wall-clock deadline.
func TestItemInstancesSaveDeadlineStopsUnattemptedChunksButKeepsEarlierCommits(t *testing.T) {
	flusher := &chunkTrackingFlusher{}
	instances := NewItemInstances(flusher, item.NewTable(nil), nil)

	const total = 2 * ItemInstanceSaveChunkSize
	items := addSequentialPending(instances, total)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	flusher.cancel = cancel

	err := instances.Save(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Save() error = %v, want context.Canceled", err)
	}

	calls := flusher.calls()
	if len(calls) != 1 {
		t.Fatalf("Flush called %d times, want exactly 1: the second chunk must never be attempted once ctx is canceled", len(calls))
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

// TestItemInstancesSaveGivesEachChunkAFreshTimeout pins the change this
// commit is actually about: each chunk's UpdateItems call gets its own
// context.WithTimeout(ctx, ItemInstanceSaveTimeout), not the outer ctx
// reused as-is. A prior version of this test suite passed even with that
// line reverted to reusing ctx directly, because no assertion looked at the
// ctx each chunk actually received. This one does: with two chunks and an
// outer ctx that carries no deadline of its own, each recorded call must
// still have a deadline (proving a per-chunk timeout was applied at all),
// and the second chunk's deadline must be strictly later than the first's
// (proving it's a fresh clock started at that chunk's own call time, not a
// deadline computed once up front). Reverting to a shared ctx would make
// both calls report the same (here: no) deadline and fail this assertion.
func TestItemInstancesSaveGivesEachChunkAFreshTimeout(t *testing.T) {
	flusher := &chunkTrackingFlusher{}
	instances := NewItemInstances(flusher, item.NewTable(nil), nil)
	addSequentialPending(instances, 2*ItemInstanceSaveChunkSize)

	if err := instances.Save(context.Background()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	deadlines := flusher.calledDeadlines()
	if len(deadlines) != 2 {
		t.Fatalf("Flush called %d times, want 2", len(deadlines))
	}
	for idx, d := range deadlines {
		if !d.ok {
			t.Fatalf("chunk %d's ctx carried no deadline, want each chunk bounded by its own ItemInstanceSaveTimeout", idx)
		}
	}
	if !deadlines[1].at.After(deadlines[0].at) {
		t.Fatalf("chunk 2's deadline (%v) is not after chunk 1's (%v): chunks are not getting independent, freshly-started budgets",
			deadlines[1].at, deadlines[0].at)
	}
}

// TestItemInstancesSaveReturnsOnCtxWhileLaneIsBackedUp holds an owner's
// persistence lane and runs a Save whose ctx is already cancelled. Save must
// return on ctx instead of waiting for the lane, and the owner job that runs
// after release must skip its write and leave the item pending for a later
// Save.
func TestItemInstancesSaveReturnsOnCtxWhileLaneIsBackedUp(t *testing.T) {
	worker := persist.New(zerolog.Nop())
	defer worker.Close(context.Background())
	flusher := &chunkTrackingFlusher{}
	instances := NewItemInstances(flusher, item.NewTable(nil), worker)
	inst := &item.Instance{ObjectID: 1, TemplateID: 1, OwnerID: 7, Count: 1, Location: item.LocationInventory}
	instances.Add(inst)

	release := make(chan struct{})
	worker.Enqueue(inst.OwnerID, func() { <-release })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() { done <- instances.Save(ctx) }()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Save() error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("Save waited on the held lane past its cancelled ctx")
	}

	close(release)
	if err := worker.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := len(flusher.calls()); n != 0 {
		t.Fatalf("Flush called %d times, want 0: the owner job ran after ctx ended", n)
	}
	if !instances.Contains(inst) {
		t.Fatal("item must stay pending after its owner job skipped the write")
	}
}

// TestItemInstancesSaveWritesInlineOnceWorkerIsClosed is the shutdown drain's
// last step: after the worker has closed, Save still writes pending items.
func TestItemInstancesSaveWritesInlineOnceWorkerIsClosed(t *testing.T) {
	worker := persist.New(zerolog.Nop())
	if err := worker.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	flusher := &chunkTrackingFlusher{}
	instances := NewItemInstances(flusher, item.NewTable(nil), worker)
	inst := &item.Instance{ObjectID: 1, TemplateID: 1, OwnerID: 7, Count: 1, Location: item.LocationInventory}
	instances.Add(inst)

	if err := instances.Save(context.Background()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if n := len(flusher.calls()); n != 1 {
		t.Fatalf("Flush called %d times, want 1", n)
	}
	if instances.Contains(inst) {
		t.Fatal("written item must leave pending")
	}
}
