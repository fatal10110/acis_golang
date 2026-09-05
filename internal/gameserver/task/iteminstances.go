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
	// ItemInstanceSaveTimeout bounds one persistence flush so a hung DB
	// cannot wedge the ticker, and then shutdown's StopAndWait, indefinitely.
	ItemInstanceSaveTimeout = 10 * time.Second
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

// Start launches the fixed item persistence task.
func (i *ItemInstances) Start(log zerolog.Logger) *scheduler.Ticker {
	return scheduler.Start(ItemInstanceTick, func() {
		ctx, cancel := context.WithTimeout(context.Background(), ItemInstanceSaveTimeout)
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

// Save flushes every pending item. The pending map is swapped out before
// the flush so a concurrent Add during I/O lands in the new map and is not
// dropped when the flush succeeds. UpdateItems is all-or-nothing; on error
// inflight ids are merged back so the next tick or shutdown flush retries
// them, except ids RemoveItems dropped during the flush — those already got
// their own successful write and must not be resurrected.
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
	err := i.UpdateItems(ctx, items)

	i.mu.Lock()
	removed := i.removedInflight
	i.removedInflight = nil
	if err != nil {
		for id, inst := range inflight {
			if _, wasRemoved := removed[id]; wasRemoved {
				continue
			}
			if _, ok := i.pending[id]; !ok {
				i.pending[id] = inst
			}
		}
	}
	i.mu.Unlock()

	return err
}

// UpdateItems persists the provided item instances immediately, as one
// atomic flush: either every row lands, or, on error, none of them do. A
// non-nil error means nothing was written, so callers must keep their
// items pending for a retry rather than dropping them.
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
