package task

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
)

// panickingItemFlusher panics on every Flush, standing in for any panic
// raised inside a Save owner job — a nil template lookup in addToBatch, a
// driver fault in the real store. Such a panic is survivable by policy (the
// lane recovers a queued job, persist.Worker.runJob), which is exactly what
// makes the job's own bookkeeping (finishOwner) its responsibility to run.
type panickingItemFlusher struct{}

func (panickingItemFlusher) Flush(context.Context, item.FlushBatch) error {
	panic("flush blew up")
}

// poisonItemFlusher panics only for poisonID and records the ids every other
// flush wrote, so a multi-owner Save can be checked for whether the owners
// dispatched after the panicking one still reached the database.
type poisonItemFlusher struct {
	poisonID int32

	mu    sync.Mutex
	saved []int32
}

func (p *poisonItemFlusher) Flush(_ context.Context, batch item.FlushBatch) error {
	for _, st := range batch.Saves {
		if st.ObjectID == p.poisonID {
			panic("flush blew up")
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, st := range batch.Saves {
		p.saved = append(p.saved, st.ObjectID)
	}
	return nil
}

func (p *poisonItemFlusher) savedIDs() []int32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.saved)
}

// closedWorker returns a persist.Worker that refuses every job, which is what
// makes Save take its inline dispatch path — the one drainItemInstances uses
// for the shutdown drain's post-Close, last-chance save.
func closedWorker(t *testing.T) *persist.Worker {
	t.Helper()
	worker := persist.New(zerolog.Nop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := worker.Close(ctx); err != nil {
		t.Fatalf("worker.Close() error = %v", err)
	}
	return worker
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
	instances := NewItemInstances(panickingItemFlusher{}, templates, worker, nil, zerolog.Nop())
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

// TestItemInstancesSaveInlinePanicKeepsDispatchingOwners covers the other
// dispatch path: when Enqueue refuses the job (a closed worker, as on the
// shutdown drain's post-Close save, or a nil one) Save runs it on its own
// goroutine, where no lane recover stands behind it. A panic escaping there
// unwound out of the dispatch loop, so every owner sorted after the
// panicking one was never dispatched at all — its entries already swapped
// out of pending, with nothing left to merge them back — and the round stayed
// in i.rounds, where its inflight copy keeps ContainsID answering true for
// ids no later Save will ever write.
func TestItemInstancesSaveInlinePanicKeepsDispatchingOwners(t *testing.T) {
	flusher := &poisonItemFlusher{poisonID: 1}
	templates := item.NewTable([]*item.Template{{ID: 10}})
	instances := NewItemInstances(flusher, templates, closedWorker(t), nil, zerolog.Nop())
	// Lane keys are the owner ids and Save dispatches them sorted, so owner
	// 100 panics while owner 200 is still undispatched behind it.
	poisoned := &item.Instance{ObjectID: 1, TemplateID: 10, OwnerID: 100, Count: 5, Location: item.LocationInventory}
	behind := &item.Instance{ObjectID: 2, TemplateID: 10, OwnerID: 200, Count: 5, Location: item.LocationInventory}
	instances.Add(poisoned)
	instances.Add(behind)

	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Save() panicked out to its caller (%v); an inline panic must not unwind the dispatch loop or the fx stop hook", r)
			}
		}()
		err = instances.Save(context.Background())
	}()

	if !errors.Is(err, errSaveJobPanic) {
		t.Fatalf("Save() error = %v, want one wrapping errSaveJobPanic", err)
	}
	if got := flusher.savedIDs(); !slices.Contains(got, behind.ObjectID) {
		t.Fatalf("saved ids = %v, want the owner behind the panicking one (%d) written", got, behind.ObjectID)
	}
	instances.mu.RLock()
	_, poisonedPending := instances.pending[poisoned.ObjectID]
	leaked := len(instances.rounds)
	instances.mu.RUnlock()
	if !poisonedPending {
		t.Fatalf("panicked owner's item in pending = false, want true so a later Save retries it")
	}
	if leaked != 0 {
		t.Fatalf("leaked rounds = %d, want 0", leaked)
	}
}

// TestItemInstancesShutdownDrainSurvivesPanickingFlush pins the drain
// sequence drainItemInstances runs (cmd/gameserver/tasks.go): save, close the
// worker, save again. Merging a panicked flush's items back to pending is what
// gives that post-Close save something to re-attempt, and it re-attempts it
// inline — so without the job's own recover the panic leaves Save, leaves
// drainItemInstances, and leaves the fx OnStop hook, which no
// fx.RecoverFromPanics converts, skipping every stop hook ordered after it.
// The reference cannot fail this way: ItemInstanceTaskManager.updateItems
// catches Exception around the whole batch and clears unconditionally, so its
// shutdown-triggered call never throws out.
func TestItemInstancesShutdownDrainSurvivesPanickingFlush(t *testing.T) {
	worker := persist.New(zerolog.Nop())
	templates := item.NewTable([]*item.Template{{ID: 10}})
	instances := NewItemInstances(panickingItemFlusher{}, templates, worker, nil, zerolog.Nop())
	instances.Add(&item.Instance{ObjectID: 1, TemplateID: 10, OwnerID: 100, Count: 5, Location: item.LocationInventory})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := instances.Save(ctx); !errors.Is(err, errSaveJobPanic) {
		t.Fatalf("first Save() error = %v, want one wrapping errSaveJobPanic", err)
	}
	if err := worker.Close(ctx); err != nil {
		t.Fatalf("worker.Close() error = %v", err)
	}

	// The post-Close save dispatches inline; nothing recovers above it.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("post-Close Save() panicked out (%v); that panic would leave the fx stop hook and skip every later one", r)
		}
	}()
	if err := instances.Save(ctx); !errors.Is(err, errSaveJobPanic) {
		t.Fatalf("post-Close Save() error = %v, want one wrapping errSaveJobPanic", err)
	}
}

// failThenPanicFlusher fails the flush for failID and panics for panicID, so a
// multi-owner round can be driven into the state where the panicking owner is
// not the one that wins round.err.
type failThenPanicFlusher struct {
	failID  int32
	panicID int32
}

var errFlushDown = errors.New("db is down")

func (f failThenPanicFlusher) Flush(_ context.Context, batch item.FlushBatch) error {
	for _, st := range batch.Saves {
		if st.ObjectID == f.panicID {
			panic("poison: flush blew up")
		}
	}
	for _, st := range batch.Saves {
		if st.ObjectID == f.failID {
			return errFlushDown
		}
	}
	return nil
}

// syncBuffer collects log output; a round's jobs can run on several lanes, so
// the writer has to be safe for concurrent use even though this test's closed
// worker keeps them on one goroutine.
type syncBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

// TestItemInstancesSavePanicIsLoggedEvenWhenAnotherOwnerWinsTheError pins the
// panic's one unconditional record. Recovering inside the job took the panic
// away from the lane, whose own "persist: recovered panic in job" line fired
// no matter what, and round.err is not a replacement for it: finishOwner keeps
// only the round's first non-nil error, and a round holds one job per owner, so
// any other owner's DB error claims that slot instead. (Save returning on
// ctx.Done is the second way, and that branch never reads round.err at all.)
// Without the log at the recover, a panicking flush would be completely silent.
// #2403 asks for the panic to be a failed round *and* a log line.
func TestItemInstancesSavePanicIsLoggedEvenWhenAnotherOwnerWinsTheError(t *testing.T) {
	var logged syncBuffer
	templates := item.NewTable([]*item.Template{{ID: 10}})
	// A closed worker dispatches inline in sorted lane-key order, which fixes
	// which owner reports first without depending on lane scheduling: owner
	// 100 takes round.err with its DB error, owner 200 panics behind it.
	instances := NewItemInstances(
		failThenPanicFlusher{failID: 1, panicID: 2},
		templates,
		closedWorker(t),
		nil,
		zerolog.New(&logged),
	)
	instances.Add(&item.Instance{ObjectID: 1, TemplateID: 10, OwnerID: 100, Count: 5, Location: item.LocationInventory})
	instances.Add(&item.Instance{ObjectID: 2, TemplateID: 10, OwnerID: 200, Count: 5, Location: item.LocationInventory})

	err := instances.Save(context.Background())

	// Documents the limit rather than wishing it away: the caller's error is
	// the round's first one, so it does not carry the panic.
	if !errors.Is(err, errFlushDown) {
		t.Fatalf("Save() error = %v, want the first owner's flush error", err)
	}
	if errors.Is(err, errSaveJobPanic) {
		t.Fatalf("Save() error = %v; round.err is first-wins, so this test no longer covers the case it was written for", err)
	}
	if got := logged.String(); !strings.Contains(got, "poison: flush blew up") {
		t.Fatalf("panic reason absent from the log; a panicking flush must never be silent.\nlog = %s", got)
	}
}
