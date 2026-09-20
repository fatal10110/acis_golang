package task

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
)

const (
	// ItemInstanceTick is the fixed cadence for lazy item persistence.
	ItemInstanceTick = time.Minute
	// ItemInstanceSaveTimeout bounds one Save call: both the periodic
	// tick's outer ctx (Start) and the shutdown hook's outer ctx
	// (cmd/gameserver/tasks.go) wrap with this same constant, and Save
	// gives each chunk its own fresh budget derived from it (see Save) so
	// a hung DB cannot wedge the ticker, and then shutdown's StopAndWait,
	// past this bound.
	//
	// This is a real coupling, not just a shared default: raising it to
	// give a chunk more room also raises how long the ticker's OnStop hook
	// can block (scheduler.Ticker.StopAndWait has no ctx of its own — see
	// Start) inside cmd/gameserver's gameServerStopTimeout budget for the
	// whole shutdown sequence. cmd/gameserver/main_core_test.go pins
	// ItemInstanceSaveTimeout staying comfortably under that budget;
	// check it before changing either constant.
	ItemInstanceSaveTimeout = 10 * time.Second
	// ItemInstanceSaveChunkSize bounds how many items one Save transaction
	// covers. Save commits chunks independently, so a batch that grew past
	// what fits in one ItemInstanceSaveTimeout window still makes monotonic
	// progress each tick instead of retrying the whole thing and never
	// converging (see the constant's use in Save).
	//
	// No measured per-item write cost backs this number (same caveat as
	// itemFlushChunkSize in data/sql/itemflush.go, which bounds placeholder
	// count rather than time and so doesn't need one). It is kept well
	// below what a full chunk's worth of rows could plausibly take inside
	// ItemInstanceSaveTimeout, so that a degraded-but-not-hung DB has room
	// to actually commit a chunk rather than losing the whole ceiling to a
	// transaction that was too big to ever finish in time; see Save's doc
	// for the residual risk this doesn't remove.
	ItemInstanceSaveChunkSize = 100
)

// ItemFlusher atomically persists one flush batch: either every change in
// it lands, or, on error, none of it does.
type ItemFlusher interface {
	Flush(ctx context.Context, batch item.FlushBatch) error
}

// ItemInstances lazily persists changed item instances.
//
// mu guards pending. Mutable item fields are guarded by item.Instance.
type ItemInstances struct {
	flusher   ItemFlusher
	templates *item.Table
	worker    *persist.Worker
	// writes orders each row's write against the same row's writes from
	// other lanes — a handler's single-row write, another owner's flush —
	// so the row keeps whichever was produced last (persist.Order).
	writes *persist.Order

	mu      sync.RWMutex
	pending map[int32]pendingItem
	// rounds holds every Save whose owner jobs have not all run yet. Each
	// records the ids RemoveItems dropped while it was outstanding, so its
	// failed items are not merged back over a container that already tore
	// down and wrote its own final state (see RemoveItems and Save).
	rounds map[*saveRound]struct{}
}

// pendingItem is one changed instance waiting for the next flush, with the
// owner its row belongs to.
//
// ownerID is remembered rather than read from the instance at flush time,
// because a destroy has already taken the instance through
// item.Instance.DestroyState, which zeroes OwnerID along with the count. A
// flush that keyed the lane off that snapshot would send every destroyed
// item's delete to the lane of owner 0, while the same row's earlier writes
// sit on its real owner's lane — exactly the split laneKey exists to
// prevent. The last owner seen for the object wins, so an item that changes
// hands before the flush is written on the new owner's lane.
type pendingItem struct {
	inst    *item.Instance
	ownerID int32
}

// saveRound is one Save's outstanding owner jobs. Its fields are guarded by
// ItemInstances.mu.
type saveRound struct {
	removed   map[int32]struct{}
	remaining int
	err       error
	done      chan struct{}
}

// NewItemInstances returns an empty item persistence task whose writes run
// on worker's lanes. A nil worker writes on the calling goroutine.
func NewItemInstances(flusher ItemFlusher, templates *item.Table, worker *persist.Worker, writes *persist.Order) *ItemInstances {
	if templates == nil {
		templates = item.NewTable(nil)
	}
	return &ItemInstances{
		flusher:   flusher,
		templates: templates,
		worker:    worker,
		writes:    writes,
		pending:   make(map[int32]pendingItem),
		rounds:    make(map[*saveRound]struct{}),
	}
}

// Start launches the fixed item persistence task. The tick's outer ctx is
// bounded by ItemInstanceSaveTimeout, the same constant the shutdown hook
// uses (cmd/gameserver/tasks.go) — not a longer, tick-only ceiling: this
// budget also bounds how long the ticker's own OnStop can block a shutdown
// in progress (scheduler.Ticker.StopAndWait has no ctx of its own, so it
// simply waits for whatever Save call is currently in flight), and that
// wait has to fit inside cmd/gameserver's gameServerStopTimeout alongside
// every other stop hook, including the final Save. A longer per-tick
// budget would drain more of a backlog per tick, but only by taking that
// same risk away from shutdown; see ItemInstanceSaveTimeout's doc.
func (i *ItemInstances) Start(log zerolog.Logger) *scheduler.Ticker {
	return scheduler.Start(ItemInstanceTick, func() {
		ctx, cancel := context.WithTimeout(context.Background(), ItemInstanceSaveTimeout)
		defer cancel()
		if err := i.Save(ctx); err != nil {
			log.Error().Err(err).Msg("task: save item instances")
		}
	}, log)
}

// Add registers inst for the next persistence tick, remembering the owner its
// row currently belongs to. A destroy reports the instance with its owner
// already zeroed, so a previously recorded owner is kept (see pendingItem);
// AddOwned is the form that does not have to guess.
func (i *ItemInstances) Add(inst *item.Instance) {
	if inst == nil {
		return
	}
	i.AddOwned(inst.Snapshot().OwnerID, inst)
}

// AddOwned registers inst for the next persistence tick as ownerID's row.
// The container holding an item knows that owner even when the item itself no
// longer does, which is exactly what a destroy reports, so this is the form
// the per-container persister hook uses: the recorded owner then survives a
// flush swapping the pending set out, and a restored item whose first
// mutation is its destruction still names an owner.
func (i *ItemInstances) AddOwned(ownerID int32, inst *item.Instance) {
	if inst == nil {
		return
	}
	i.mu.Lock()
	entry := i.pending[inst.ObjectID]
	entry.inst = inst
	if ownerID != 0 {
		entry.ownerID = ownerID
	}
	i.pending[inst.ObjectID] = entry
	i.mu.Unlock()
}

// Contains reports whether inst's object id is currently pending.
func (i *ItemInstances) Contains(inst *item.Instance) bool {
	if inst == nil {
		return false
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	_, ok := i.pending[inst.ObjectID]
	return ok
}

// RemoveItems removes every provided item from the pending set. If a Save
// flush is currently in progress, the removed ids are also recorded so that
// flush's merge-back does not put them back on error: this container tore
// down and wrote its own final state (network.flushItemPersistence), and
// that write must not be undone by a stale inflight copy.
func (i *ItemInstances) RemoveItems(items []*item.Instance) {
	i.mu.Lock()
	defer i.mu.Unlock()
	for _, inst := range items {
		if inst == nil {
			continue
		}
		delete(i.pending, inst.ObjectID)
		for r := range i.rounds {
			r.removed[inst.ObjectID] = struct{}{}
		}
	}
}

// Save flushes every pending item in chunks of at most
// ItemInstanceSaveChunkSize, each committed by its own UpdateItems call
// under its own fresh ItemInstanceSaveTimeout (bounded by whatever remains
// of ctx). Items are grouped by owner and each owner's chunks run on that
// owner's persistence lane. Save returns once every owner's job has run, or
// when ctx ends: an owner job still queued then skips its writes when it runs
// and puts its items back to pending, so the wait never outlasts ctx. Once
// the worker is closed, Save writes on the calling goroutine. The
// pending map is swapped out before the flush so a concurrent
// Add during I/O lands in the new map and is not dropped when the flush
// succeeds. Chunking makes progress monotonic: a batch that has grown past
// what one ItemInstanceSaveTimeout window can write still gets its earlier
// chunks committed, and only the chunks that failed or were never attempted
// (ctx already expired) go back to pending. A single unbounded flush would
// retry the whole growing batch every tick and never converge. On error,
// ids RemoveItems dropped during the flush are not merged back — those
// already got their own successful write and must not be resurrected.
//
// Save no longer gives callers UpdateItems's whole-batch atomicity: two
// items that must land together (e.g. both legs of a trade reaching pending
// through itemcontainer's SetItemPersister hook) can now fall on either
// side of a chunk boundary and commit, or fail, independently. This
// narrows what a Save tick previously promised, but not past what the
// Java reference already does: ItemInstanceTaskManager.updateItems commits
// five sequential executeBatch calls on an autocommit connection with no
// transaction at all, so per-statement partial visibility on error is
// already the oracle's behavior, at a finer grain than one chunk here.
// flushItemPersistence (network/lifecycle.go) is the caller that still
// needs, and gets, whole-container atomicity: it calls UpdateItems
// directly for one container's items, bypassing Save's chunking entirely.
//
// items is sorted by ObjectID before chunking purely to fix which items
// land in which chunk deterministically (tests rely on this); it is not an
// ordering guarantee for callers and must not be read as a write-priority
// policy.
//
// Residual risk this does not remove: ItemInstanceSaveChunkSize and
// ItemInstanceSaveTimeout are a fixed, unmeasured size:time ratio. A chunk
// whose real write cost exceeds that ratio still fails every attempt at
// that size, the same way the pre-chunking flush did at the whole-batch
// size — chunking only lowers how much of one tick's ItemInstanceSaveTimeout
// budget such a chunk can waste, it does not guarantee any given chunk fits
// its budget.
//
// One Save call's total DB-write time is still capped near
// ItemInstanceSaveTimeout, same as before chunking existed — chunking buys
// smaller, independently-committing units inside that one window, not a
// bigger window. A backlog that needs more than that per tick still only
// drains it gradually, one tick's worth at a time. Widening the per-tick
// budget itself needs the flush to be cancelable when shutdown starts
// (today's Start/StopAndWait pair cannot do that — see Start's doc); until
// that exists, a longer per-tick ceiling would trade shutdown safety for
// throughput instead of buying both.
//
// Concurrent callers of Add and RemoveItems are safe. A Save may overlap
// the owner jobs of an earlier one that returned on its ctx; each keeps its
// own record of removed ids. The shutdown hook is appended before the
// ticker's, so fx's reverse stop order runs the final Save only after the
// ticker has stopped.
func (i *ItemInstances) Save(ctx context.Context) error {
	i.mu.Lock()
	inflight := i.pending
	i.pending = make(map[int32]pendingItem)
	// Each owner's items are written on that owner's persistence lane, so
	// they can never interleave with a container flush for the same owner
	// (network.flushItemPersistence) and land an older snapshot after it.
	byOwner := make(map[int32][]pendingItem)
	for _, entry := range inflight {
		key := i.laneKey(entry.inst.Snapshot(), entry.ownerID)
		byOwner[key] = append(byOwner[key], entry)
	}
	round := &saveRound{removed: make(map[int32]struct{}), remaining: len(byOwner), done: make(chan struct{})}
	if round.remaining == 0 {
		i.mu.Unlock()
		return nil
	}
	i.rounds[round] = struct{}{}
	i.mu.Unlock()

	for _, owner := range slices.Sorted(maps.Keys(byOwner)) {
		entries := byOwner[owner]
		// Fixes chunk boundaries so they don't depend on map iteration
		// order; see the chunk-boundary note above.
		slices.SortFunc(entries, func(a, b pendingItem) int { return cmp.Compare(a.inst.ObjectID, b.inst.ObjectID) })
		job := func() {
			failed, err := i.saveChunks(ctx, entries)
			i.finishOwner(round, failed, err)
		}
		// A closed worker has already run every job it accepted, so writing
		// here cannot land ahead of an older queued write.
		if !i.worker.Enqueue(owner, job) {
			job()
		}
	}

	select {
	case <-round.done:
	case <-ctx.Done():
		// The owner jobs still queued fail fast once they run and merge
		// their items back to pending for a later Save.
		select {
		case <-round.done:
		default:
			return ctx.Err()
		}
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	return round.err
}

// finishOwner merges one owner job's failed items back to pending, skipping
// any RemoveItems dropped while round was outstanding, and closes round once
// its last owner job has run.
func (i *ItemInstances) finishOwner(round *saveRound, failed []pendingItem, err error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	for _, entry := range failed {
		if _, wasRemoved := round.removed[entry.inst.ObjectID]; wasRemoved {
			continue
		}
		// Keeps the owner this round resolved: a retry of a destroyed item
		// must not fall back to the zeroed owner on its instance.
		if _, ok := i.pending[entry.inst.ObjectID]; !ok {
			i.pending[entry.inst.ObjectID] = entry
		}
	}
	if round.err == nil {
		round.err = err
	}
	round.remaining--
	if round.remaining == 0 {
		delete(i.rounds, round)
		close(round.done)
	}
}

// laneKey picks the persistence lane an item's write runs on: its owner's,
// except a destroyed pet collar, whose write also deletes its pets row. That
// one runs on the collar's own lane, where every pets-row save is queued, so
// a save still waiting there cannot land after the delete and recreate the
// row. The same argument is why every other destroyed item's write stays on
// its owner's lane, which is where that row's earlier writes are: ownerID
// comes from the pending entry, not from the instance, whose owner a destroy
// has already zeroed (see pendingItem).
func (i *ItemInstances) laneKey(st item.InstanceState, ownerID int32) int32 {
	if st.Count <= 0 {
		if tmpl, _ := i.templates.Get(st.TemplateID); isPetCollar(tmpl) {
			return st.ObjectID
		}
	}
	return ownerID
}

// saveChunks writes items in chunks of at most ItemInstanceSaveChunkSize,
// each under its own ItemInstanceSaveTimeout, and returns the items whose
// chunk failed or was never attempted because ctx had already ended.
func (i *ItemInstances) saveChunks(ctx context.Context, entries []pendingItem) ([]pendingItem, error) {
	var firstErr error
	var failed []pendingItem
	for chunk := range slices.Chunk(entries, ItemInstanceSaveChunkSize) {
		if ctx.Err() != nil {
			failed = append(failed, chunk...)
			if firstErr == nil {
				firstErr = ctx.Err()
			}
			continue
		}
		items := make([]*item.Instance, 0, len(chunk))
		for _, entry := range chunk {
			items = append(items, entry.inst)
		}
		chunkCtx, cancel := context.WithTimeout(ctx, ItemInstanceSaveTimeout)
		err := i.UpdateItems(chunkCtx, items)
		cancel()
		if err != nil {
			failed = append(failed, chunk...)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return failed, firstErr
}

// UpdateItems persists the provided item instances immediately, as one
// atomic flush: either every row lands, or, on error, none of them do. A
// non-nil error means nothing was written, so callers must keep their
// items pending for a retry rather than dropping them.
//
// This per-call guarantee is unchanged by Save's chunking (Save simply
// calls UpdateItems once per chunk); network.flushItemPersistence relies on
// it directly, calling UpdateItems for one container's items outside of
// Save, and needs it to stay whole-batch atomic. Do not chunk that call
// site too without checking its callers still get what they need.
func (i *ItemInstances) UpdateItems(ctx context.Context, items []*item.Instance) error {
	if len(items) == 0 {
		return nil
	}
	if i.flusher == nil {
		return errors.New("task: item persistence is nil")
	}

	items = slices.DeleteFunc(items, func(inst *item.Instance) bool { return inst == nil })
	slices.SortFunc(items, func(a, b *item.Instance) int { return cmp.Compare(a.ObjectID, b.ObjectID) })

	// Each row's state and its place in that row's write order are taken
	// together, under the instance: the state this flush will land is fixed
	// here, not when the write finally runs, so a single-row write produced
	// after it always holds the later place — and one produced before it the
	// earlier, however long this flush then waits for the rows. Reading the
	// state later than the place is what let a flush hold an older place
	// while carrying a newer state, and an older write then landed on top of
	// its delete.
	write := i.writes.Begin()
	states := make([]item.InstanceState, 0, len(items))
	for _, inst := range items {
		inst.WithState(func(st item.InstanceState) {
			write.Add(st.ObjectID)
			states = append(states, st)
		})
	}
	var err error
	write.Run(func(keep []int32) {
		var batch item.FlushBatch
		for _, st := range states {
			if _, found := slices.BinarySearch(keep, st.ObjectID); !found {
				continue
			}
			i.addToBatch(&batch, st)
		}
		err = i.flusher.Flush(ctx, batch)
	})
	return err
}

// addToBatch resolves st's persistence effect and appends it to batch,
// matching the per-item semantics updateItem used to apply immediately:
// delete when count <= 0 or location == VOID, augmentation delete/save
// only for weapons, pet-row delete only for a pet collar at zero count.
//
// It takes the state rather than the instance because the state is read where
// the write's place in its row's order is taken (UpdateItems), not here.
func (i *ItemInstances) addToBatch(batch *item.FlushBatch, st item.InstanceState) {
	tmpl, _ := i.templates.Get(st.TemplateID)
	isWeapon := tmpl != nil && tmpl.Kind == item.KindWeapon

	if st.Count <= 0 || st.Location == item.LocationVoid {
		batch.Deletes = append(batch.Deletes, st.ObjectID)
		if st.Count <= 0 {
			if isWeapon {
				batch.AugmentationDeletes = append(batch.AugmentationDeletes, st.ObjectID)
			}
			if isPetCollar(tmpl) {
				batch.PetDeletes = append(batch.PetDeletes, st.ObjectID)
			}
		}
		return
	}

	batch.Saves = append(batch.Saves, st)
	if isWeapon {
		if st.Augmentation == nil {
			batch.AugmentationDeletes = append(batch.AugmentationDeletes, st.ObjectID)
		} else {
			batch.AugmentationSaves = append(batch.AugmentationSaves, item.FlushAugmentationSave{
				ObjectID:     st.ObjectID,
				Augmentation: *st.Augmentation,
			})
		}
	}
}

func isPetCollar(tmpl *item.Template) bool {
	return tmpl != nil && tmpl.EtcItem != nil && tmpl.EtcItem.Type == item.EtcItemPetCollar
}
