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
//
// OwnerID is the owner the item's row holds at the moment the action is
// produced. It is the ordering key for the write: every write of one row has
// to run behind the previous one, and a PersistDelete carries no instance to
// read that owner from later.
type Persist struct {
	Action   PersistAction
	Item     *item.Instance
	ObjectID int32
	OwnerID  int32
}

// Result carries side effects common to inventory workflows.
type Result struct {
	Persist          []Persist
	EquipmentChanged bool
	// Changed lists every instance whose equip state (paperdoll occupancy)
	// this call altered — the caller resolves each one's current Equipped()
	// state after the call to decide whether to attach or detach its
	// template's stat functions.
	Changed []*item.Instance
}

// Save returns a persistence action for a new item row.
func Save(inst *item.Instance) Persist {
	return Persist{Action: PersistSave, Item: inst}
}

// Update returns a persistence action for an existing item row.
func Update(inst *item.Instance) Persist {
	return Persist{Action: PersistUpdate, Item: inst}
}

// Delete returns a persistence action for deleting an item row. ownerID is
// the owner the row currently holds, not whoever caused the delete.
func Delete(ownerID, objectID int32) Persist {
	return Persist{Action: PersistDelete, ObjectID: objectID, OwnerID: ownerID}
}

// DestroyedOrUpdated returns delete when inst is fully consumed, otherwise
// update.
//
// ownerID is the owner the row held before the destroy, which the caller has
// to read before it destroys: a fully consumed instance has already been
// through item.Instance.DestroyState, which zeroes OwnerID along with the
// count, so the delete cannot recover the lane its earlier writes used from
// inst itself.
func DestroyedOrUpdated(ownerID int32, inst *item.Instance) Persist {
	if inst == nil {
		return Persist{}
	}
	st := inst.Snapshot()
	if st.Count == 0 {
		return Delete(ownerID, st.ObjectID)
	}
	return Update(inst)
}
