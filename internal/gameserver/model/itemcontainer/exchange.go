package itemcontainer

import "github.com/fatal10110/acis_golang/internal/gameserver/model/item"

// Held is an inventory Exchange holds locked. Its reads see contents nothing
// else can change until Exchange returns, and its Transfer is the only way to
// change them meanwhile.
type Held struct {
	inv *Inventory
}

// Moved is what one Held.Transfer did.
type Moved struct {
	// Item is the instance the receiver now holds: the source instance
	// itself, the receiver's stack it merged into, or a new instance.
	Item *item.Instance
	// Remaining is the source instance still holding the units not moved,
	// or nil when the source instance left the sender.
	Remaining *item.Instance
	// FreedObjectID is the source instance's id when that instance was
	// merged into the receiver's stack and no longer exists.
	FreedObjectID int32
	// Created reports that Item is a new instance under the newObjectID the
	// move was given.
	Created bool
}

// Exchange runs fn with both inventories locked, the lower owner id first, so
// a check fn makes and every Transfer after it see the same contents: neither
// owner can drop, use, equip or be handed anything in between. This is the
// one place two inventories are locked together. Queued update deliveries run
// after both are released. a and b must be different inventories.
func Exchange(a, b *Inventory, fn func(a, b Held)) {
	first, second := a, b
	if second.OwnerID() < first.OwnerID() {
		first, second = second, first
	}
	func() {
		first.Container.mu.Lock()
		defer first.Container.mu.Unlock()
		second.Container.mu.Lock()
		defer second.Container.mu.Unlock()
		first.mu.Lock()
		defer first.mu.Unlock()
		second.mu.Lock()
		defer second.mu.Unlock()
		fn(Held{inv: a}, Held{inv: b})
	}()
	a.fireDelivery()
	b.fireDelivery()
}

// ItemByObjectID returns the instance identified by objectID, or nil.
func (h Held) ItemByObjectID(objectID int32) *item.Instance {
	return h.inv.items[objectID]
}

// ItemByTemplateID returns the first instance of templateID, or nil.
func (h Held) ItemByTemplateID(templateID int32) *item.Instance {
	return h.inv.itemByTemplateIDLocked(templateID)
}

// Templates returns the template table the inventory resolves items with.
func (h Held) Templates() *item.Table {
	return h.inv.templates
}

// ValidateCapacity reports whether slotCount more stacks fit, as
// Container.ValidateCapacity does.
func (h Held) ValidateCapacity(slotCount int) bool {
	if slotCount == 0 || h.inv.SlotLimit <= 0 {
		return true
	}
	return len(h.inv.items)+slotCount <= h.inv.SlotLimit
}

// ValidateWeight reports whether weight more fits, as
// Inventory.ValidateWeight does.
func (h Held) ValidateWeight(weight int) bool {
	if h.inv.WeightLimit <= 0 {
		return true
	}
	return h.inv.totalWeight+weight <= h.inv.WeightLimit
}

// Transfer moves count units of objectID from h to to, the way
// Inventory.TransferItem does, and queues both inventories' updates.
// newObjectID names the instance created when the move splits a stack into a
// receiver holding none of that item; it must be non-zero whenever that can
// happen. Transfer changes nothing when it reports false.
func (h Held) Transfer(objectID int32, count int, to Held, newObjectID int32) (Moved, bool) {
	src := h.inv.items[objectID]
	if src == nil || count <= 0 {
		return Moved{}, false
	}
	st := src.Snapshot()
	if count > st.Count {
		count = st.Count
	}
	tmpl, ok := h.inv.templates.Get(st.TemplateID)
	if !ok {
		return Moved{}, false
	}
	var stack *item.Instance
	if tmpl.Stackable {
		stack = to.inv.itemByTemplateIDLocked(st.TemplateID)
	}
	whole := st.Count == count
	if !whole && stack == nil && newObjectID == 0 {
		return Moved{}, false
	}

	var m Moved
	switch {
	case whole && stack == nil:
		delete(h.inv.items, objectID)
		to.inv.insertLocked(src)
		m.Item = src
	case whole:
		delete(h.inv.items, objectID)
		src.DestroyState()
		m.FreedObjectID = objectID
	default:
		if _, ok := src.ReduceCount(count); !ok {
			return Moved{}, false
		}
		m.Remaining = src
	}
	switch {
	case m.Item != nil:
	case stack != nil:
		stack.AddCount(count)
		m.Item = stack
	default:
		inst, _ := newInstance(h.inv.templates, st.TemplateID, count, newObjectID)
		to.inv.insertLocked(inst)
		m.Item, m.Created = inst, true
	}

	if m.Remaining != nil {
		h.inv.queueUpdateLocked(m.Remaining, UpdateModified)
	} else {
		h.inv.queueUpdateRecordLocked(objectID, st.TemplateID, count, UpdateRemoved)
	}
	if stack != nil {
		to.inv.queueUpdateLocked(m.Item, UpdateModified)
	} else {
		to.inv.queueUpdateLocked(m.Item, UpdateAdded)
	}
	return m, true
}
