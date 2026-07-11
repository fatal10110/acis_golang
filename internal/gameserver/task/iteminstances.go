package task

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// ItemInstanceTick is the fixed cadence for lazy item persistence.
const ItemInstanceTick = time.Minute

// ItemPersistence saves and deletes item rows.
type ItemPersistence interface {
	Save(context.Context, *item.Instance) error
	Delete(context.Context, int32) error
}

// AugmentationPersistence saves and deletes item augmentation rows.
type AugmentationPersistence interface {
	Save(context.Context, int32, item.Augmentation) error
	Delete(context.Context, int32) error
}

// PetItemPersistence deletes pet rows tied to consumed pet-collar items.
type PetItemPersistence interface {
	DeleteByItemObjectID(context.Context, int32) error
}

// ItemInstances lazily persists changed item instances.
//
// mu guards pending. The owning actor/runtime must serialize access to the
// item instances themselves.
type ItemInstances struct {
	items         ItemPersistence
	augmentations AugmentationPersistence
	pets          PetItemPersistence
	templates     *item.Table

	mu      sync.RWMutex
	pending map[int32]*item.Instance
}

// NewItemInstances returns an empty item persistence task.
func NewItemInstances(items ItemPersistence, augmentations AugmentationPersistence, pets PetItemPersistence, templates *item.Table) *ItemInstances {
	if templates == nil {
		templates = item.NewTable(nil)
	}
	return &ItemInstances{
		items:         items,
		augmentations: augmentations,
		pets:          pets,
		templates:     templates,
		pending:       make(map[int32]*item.Instance),
	}
}

// Start launches the fixed item persistence task.
func (i *ItemInstances) Start(log zerolog.Logger) *scheduler.Ticker {
	return scheduler.Start(ItemInstanceTick, func() {
		if err := i.Save(context.Background()); err != nil {
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

// RemoveItems removes every provided item from the pending set.
func (i *ItemInstances) RemoveItems(items []*item.Instance) {
	i.mu.Lock()
	defer i.mu.Unlock()
	for _, inst := range items {
		if inst != nil {
			delete(i.pending, inst.ObjectID)
		}
	}
}

// Save flushes every pending item and clears the pending set.
func (i *ItemInstances) Save(ctx context.Context) error {
	items := i.snapshotPending()
	err := i.UpdateItems(ctx, items)

	i.mu.Lock()
	for _, inst := range items {
		delete(i.pending, inst.ObjectID)
	}
	i.mu.Unlock()

	return err
}

// UpdateItems persists the provided item instances immediately.
func (i *ItemInstances) UpdateItems(ctx context.Context, items []*item.Instance) error {
	if len(items) == 0 {
		return nil
	}
	if i.items == nil {
		return errors.New("task: item persistence is nil")
	}

	slices.SortFunc(items, func(a, b *item.Instance) int { return cmp.Compare(a.ObjectID, b.ObjectID) })

	var errs []error
	for _, inst := range items {
		if inst == nil {
			continue
		}
		if err := i.updateItem(ctx, inst); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (i *ItemInstances) snapshotPending() []*item.Instance {
	i.mu.RLock()
	defer i.mu.RUnlock()
	items := make([]*item.Instance, 0, len(i.pending))
	for _, inst := range i.pending {
		items = append(items, inst)
	}
	return items
}

func (i *ItemInstances) updateItem(ctx context.Context, inst *item.Instance) error {
	tmpl, _ := i.templates.Get(inst.TemplateID)
	isWeapon := tmpl != nil && tmpl.Kind == item.KindWeapon

	if inst.Count <= 0 || inst.Location == item.LocationVoid {
		if err := i.items.Delete(ctx, inst.ObjectID); err != nil {
			return fmt.Errorf("delete item %d: %w", inst.ObjectID, err)
		}
		if inst.Count <= 0 {
			if isWeapon && i.augmentations != nil {
				if err := i.augmentations.Delete(ctx, inst.ObjectID); err != nil {
					return fmt.Errorf("delete augmentation %d: %w", inst.ObjectID, err)
				}
			}
			if i.pets != nil && isPetCollar(tmpl) {
				if err := i.pets.DeleteByItemObjectID(ctx, inst.ObjectID); err != nil {
					return fmt.Errorf("delete pet item %d: %w", inst.ObjectID, err)
				}
			}
		}
		return nil
	}

	if err := i.items.Save(ctx, inst); err != nil {
		return fmt.Errorf("save item %d: %w", inst.ObjectID, err)
	}
	if isWeapon && i.augmentations != nil {
		if inst.Augmentation == nil {
			if err := i.augmentations.Delete(ctx, inst.ObjectID); err != nil {
				return fmt.Errorf("delete augmentation %d: %w", inst.ObjectID, err)
			}
		} else if err := i.augmentations.Save(ctx, inst.ObjectID, *inst.Augmentation); err != nil {
			return fmt.Errorf("save augmentation %d: %w", inst.ObjectID, err)
		}
	}
	return nil
}

func isPetCollar(tmpl *item.Template) bool {
	return tmpl != nil && tmpl.EtcItem != nil && tmpl.EtcItem.Type == item.EtcItemPetCollar
}
