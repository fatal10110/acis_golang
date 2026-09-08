package task

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

const (
	// ItemInstanceTick is the fixed cadence for lazy item persistence.
	ItemInstanceTick = time.Minute
	// ItemInstanceSaveTimeout bounds one chunk's persistence transaction
	// (see ItemInstanceSaveChunkSize) so a hung DB cannot wedge the ticker,
	// and then shutdown's StopAndWait, indefinitely. Save gives each chunk
	// its own fresh ItemInstanceSaveTimeout budget rather than sharing one
	// deadline across the whole flush.
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
	// itemInstanceSaveTickCeiling bounds one periodic Save call (see Start)
	// so a backlog that needs many chunks can use most of the tick period
	// to drain — not just ItemInstanceSaveTimeout's single chunk budget —
	// while still returning in bounded time: an unbounded Save would let a
	// large-enough backlog block the next tick, and shutdown's StopAndWait,
	// indefinitely, the exact wedge ItemInstanceSaveTimeout exists to
	// prevent. The shutdown flush keeps its own, shorter, explicit budget
	// (cmd/gameserver/tasks.go) rather than this one.
	itemInstanceSaveTickCeiling = ItemInstanceTick
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

	mu      sync.RWMutex
	pending map[int32]*item.Instance
	// removedInflight is non-nil only while a Save flush is in progress. It
	// records ids RemoveItems dropped during that window so a failed flush's
	// merge-back does not resurrect a container that already tore down and
	// wrote its own final state (see RemoveItems and Save).
	removedInflight map[int32]struct{}
}

// NewItemInstances returns an empty item persistence task.
func NewItemInstances(flusher ItemFlusher, templates *item.Table) *ItemInstances {
	if templates == nil {
		templates = item.NewTable(nil)
	}
	return &ItemInstances{
		flusher:   flusher,
		templates: templates,
		pending:   make(map[int32]*item.Instance),
	}
}

// Start launches the fixed item persistence task. The tick's own budget is
// itemInstanceSaveTickCeiling, not ItemInstanceSaveTimeout: Save spends that
// budget across as many freshly-timed chunks as fit, rather than one shared
// ItemInstanceSaveTimeout window for the whole flush.
func (i *ItemInstances) Start(log zerolog.Logger) *scheduler.Ticker {
	return scheduler.Start(ItemInstanceTick, func() {
		ctx, cancel := context.WithTimeout(context.Background(), itemInstanceSaveTickCeiling)
		defer cancel()
		if err := i.Save(ctx); err != nil {
			log.Error().Err(err).Msg("task: save item instances")
		}
	}, log)
}

// Add registers inst for the next persistence tick.
func (i *ItemInstances) Add(inst *item.Instance) {
	if inst == nil {
		return
	}
	i.mu.Lock()
	i.pending[inst.ObjectID] = inst
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
		if i.removedInflight != nil {
			i.removedInflight[inst.ObjectID] = struct{}{}
		}
	}
}

// Save flushes every pending item in chunks of at most
// ItemInstanceSaveChunkSize, each committed by its own UpdateItems call
// under its own fresh ItemInstanceSaveTimeout (bounded by whatever remains
// of ctx). The pending map is swapped out before the flush so a concurrent
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
// size — chunking only lowers how much of the tick's throughput one such
// chunk can waste, it does not guarantee any given chunk fits its budget.
//
// Concurrent callers of Add and RemoveItems are safe; concurrent Saves are
// not expected. The shutdown hook is appended before the ticker's, so fx's
// reverse stop order runs the final Save only after the ticker has stopped.
func (i *ItemInstances) Save(ctx context.Context) error {
	i.mu.Lock()
	inflight := i.pending
	i.pending = make(map[int32]*item.Instance)
	i.removedInflight = make(map[int32]struct{})
	i.mu.Unlock()

	items := make([]*item.Instance, 0, len(inflight))
	for _, inst := range inflight {
		items = append(items, inst)
	}
	// Fixes chunk boundaries so they don't depend on map iteration order;
	// see the chunk-boundary note above.
	slices.SortFunc(items, func(a, b *item.Instance) int { return cmp.Compare(a.ObjectID, b.ObjectID) })

	var firstErr error
	failed := make([]*item.Instance, 0)
	for chunk := range slices.Chunk(items, ItemInstanceSaveChunkSize) {
		if ctx.Err() != nil {
			failed = append(failed, chunk...)
			if firstErr == nil {
				firstErr = ctx.Err()
			}
			continue
		}
		chunkCtx, cancel := context.WithTimeout(ctx, ItemInstanceSaveTimeout)
		err := i.UpdateItems(chunkCtx, chunk)
		cancel()
		if err != nil {
			failed = append(failed, chunk...)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	i.mu.Lock()
	removed := i.removedInflight
	i.removedInflight = nil
	for _, inst := range failed {
		if _, wasRemoved := removed[inst.ObjectID]; wasRemoved {
			continue
		}
		if _, ok := i.pending[inst.ObjectID]; !ok {
			i.pending[inst.ObjectID] = inst
		}
	}
	i.mu.Unlock()

	return firstErr
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

	var batch item.FlushBatch
	for _, inst := range items {
		i.addToBatch(&batch, inst)
	}
	return i.flusher.Flush(ctx, batch)
}

// addToBatch resolves inst's persistence effect and appends it to batch,
// matching the per-item semantics updateItem used to apply immediately:
// delete when count <= 0 or location == VOID, augmentation delete/save
// only for weapons, pet-row delete only for a pet collar at zero count.
func (i *ItemInstances) addToBatch(batch *item.FlushBatch, inst *item.Instance) {
	st := inst.Snapshot()
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
