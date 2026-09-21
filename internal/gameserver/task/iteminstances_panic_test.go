package task

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
)

// panickingItemFlusher panics on every Flush, standing in for any panic
// raised inside a Save owner job — a nil template lookup in addToBatch, a
// driver fault in the real store. persist.Worker.runJob recovers such a
// panic on purpose so one bad job cannot kill a lane, which is exactly what
// makes the job's own bookkeeping (finishOwner) its responsibility to run.
type panickingItemFlusher struct{}

func (panickingItemFlusher) Flush(context.Context, item.FlushBatch) error {
	panic("flush blew up")
}

// TestItemInstancesSaveSurvivesPanickingFlush pins #2403: a panicking owner
// job must still complete its round. Before the fix the job's finishOwner
// call was unreachable past the panic, so the round never reached zero
// remaining — Save blocked until its own ctx expired (ItemInstanceSaveTimeout
// on every tick and on the shutdown drain), the round stayed in i.rounds for
// the rest of the process, and the items it had already swapped out of
// pending were in no map at all: never written, never retried, leaving the
// row holding pre-write state that a relog would restore.
func TestItemInstancesSaveSurvivesPanickingFlush(t *testing.T) {
	worker := persist.New(zerolog.Nop())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := worker.Close(ctx); err != nil {
			t.Errorf("worker.Close() error = %v", err)
		}
	})

	templates := item.NewTable([]*item.Template{{ID: 10}})
	instances := NewItemInstances(panickingItemFlusher{}, templates, worker, nil)
	inst := &item.Instance{ObjectID: 1, TemplateID: 10, OwnerID: 100, Count: 5, Location: item.LocationInventory}
	instances.Add(inst)

	// Well under ItemInstanceSaveTimeout: on the unfixed code Save only
	// returns when this expires, so the deadline is what separates "returned
	// promptly with an error" from "hung to its ctx".
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	err := instances.Save(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("Save() error = nil, want non-nil so the caller sees a failed round")
	}
	if ctx.Err() != nil {
		t.Fatalf("Save() hung to its ctx (%v elapsed); the round never completed", elapsed)
	}
	if !instances.Contains(inst) {
		t.Fatalf("item is pending = false, want true: a panicked flush must leave its items for a later Save")
	}
	instances.mu.RLock()
	leaked := len(instances.rounds)
	instances.mu.RUnlock()
	if leaked != 0 {
		t.Fatalf("leaked rounds = %d, want 0", leaked)
	}
}
