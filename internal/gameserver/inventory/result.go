package inventory

import "github.com/fatal10110/acis_golang/internal/gameserver/model/item"

// PersistAction identifies the item-store operation produced by a mutation.
type PersistAction uint8

const (
	// PersistNone means no store operation is needed.
	PersistNone PersistAction = iota
	// PersistSave stores a new item row.
	PersistSave
	// PersistUpdate updates an existing item row.
	PersistUpdate
	// PersistDelete deletes an item row by object id.
	PersistDelete
)

// Persist is one item-store operation produced by an inventory mutation.
type Persist struct {
	Action   PersistAction
	Item     *item.Instance
	ObjectID int32
}

// Result carries side effects common to inventory workflows.
type Result struct {
	Persist          []Persist
	EquipmentChanged bool
}

// Save returns a persistence action for a new item row.
func Save(inst *item.Instance) Persist {
	return Persist{Action: PersistSave, Item: inst}
}

// Update returns a persistence action for an existing item row.
func Update(inst *item.Instance) Persist {
	return Persist{Action: PersistUpdate, Item: inst}
}

// Delete returns a persistence action for deleting an item row.
func Delete(objectID int32) Persist {
	return Persist{Action: PersistDelete, ObjectID: objectID}
}

// DestroyedOrUpdated returns delete when inst is fully consumed, otherwise update.
func DestroyedOrUpdated(inst *item.Instance) Persist {
	if inst == nil {
		return Persist{}
	}
	if inst.Count == 0 {
		return Delete(inst.ObjectID)
	}
	return Update(inst)
}
