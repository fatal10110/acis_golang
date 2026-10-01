package itemcontainer

import (
	"slices"
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// Paperdoll equip-array positions, matching the items table's loc_data
// value for an equipped instance.
const (
	Under = iota
	LEar
	REar
	Neck
	LFinger
	RFinger
	Head
	RHand
	LHand
	Gloves
	Chest
	Legs
	Feet
	Cloak
	Face
	Hair
	HairAll
)

// UpdateState describes what changed about an item instance for a pending
// inventory-update notification.
type UpdateState uint8

// Update states. Values start at 1, not 0: the wire format's leading state
// code 0 is reserved for "unchanged" (never queued here, but the client's
// parser assigns it that meaning and shifts every other code accordingly).
const (
	updateUnchanged UpdateState = iota
	UpdateAdded
	UpdateModified
	UpdateRemoved
)

// Update is one pending inventory-change notification. Delivering it to
// the client as an actual packet is the network layer's job; Inventory
// only queues the fact that something changed.
type Update struct {
	ObjectID   int32
	TemplateID int32
	Count      int
	State      UpdateState
}

// Inventory is an equip-capable item container: a player or pet's own
// carried items, with PaperdollSlots equip-array positions layered on top
// of the plain Container behavior.
//
// WeightLimit caps the inventory's total carried weight; 0 means
// unlimited, sourced the same way Container.SlotLimit is: this package
// doesn't load config or read owner stats itself.
//
// An inventory whose owner computes its limits live (a player's, whose slot
// limit follows config and the inventoryLimit stat and whose weight limit
// follows CON, config and the weightLimit stat, or a pet's, whose weight
// limit does the same) takes that owner as its Limiter instead of the fixed
// SlotLimit and WeightLimit.
//
// A player's inventory also ties its left hand to the right: a bow or
// fishing rod leaving the right hand takes the arrows or lure out of the
// left, and a bow put in the right hand takes its matching arrows into the
// left. Every paperdoll mutation applies that rule, so each caller reports
// the extra left-hand change among the instances it altered.
//
// mu guards paperdoll, wornMask, totalWeight, updates, removed and limiter.
// Mutable item fields are guarded by item.Instance.
type Inventory struct {
	*Container

	equipLocation item.Location
	// pairsHands turns the bow/rod left-hand rule on. It is set once at
	// construction and never written again.
	pairsHands bool

	WeightLimit int

	mu          sync.Mutex
	paperdoll   [item.PaperdollSlots]*item.Instance
	wornMask    int32
	totalWeight int
	updates     []Update
	delivery    Delivery
	limiter     Limiter
	// removed holds the object ids of instances that left the inventory
	// since the last fireDelivery, which hands them to a RemovalDelivery.
	removed []int32
}

// Limiter reports how many item slots and how much carried weight an
// inventory's owner may hold right now. Both limits always apply: a weight
// limit of 0 admits no weight at all.
type Limiter interface {
	InventoryLimit() int
	WeightLimit() int
}

// SetLimiter makes limiter the source of inv's slot and weight limits,
// replacing SlotLimit and WeightLimit. A nil limiter restores them.
func (inv *Inventory) SetLimiter(limiter Limiter) {
	inv.mu.Lock()
	inv.limiter = limiter
	inv.mu.Unlock()
}

// currentLimiter returns inv's limiter, or nil when it uses its fixed
// limits.
func (inv *Inventory) currentLimiter() Limiter {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	return inv.limiter
}

// slotLimit returns inv's current slot limit and whether it has one. It
// asks the limiter without holding any inventory lock, so callers must not
// hold one either: the owner's limit may read the paperdoll.
func (inv *Inventory) slotLimit() (limit int, bounded bool) {
	if limiter := inv.currentLimiter(); limiter != nil {
		return limiter.InventoryLimit(), true
	}
	return inv.SlotLimit, inv.SlotLimit > 0
}

// weightLimit returns inv's current weight limit and whether it has one,
// under the same locking rule as slotLimit.
func (inv *Inventory) weightLimit() (limit int, bounded bool) {
	if limiter := inv.currentLimiter(); limiter != nil {
		return limiter.WeightLimit(), true
	}
	return inv.WeightLimit, inv.WeightLimit > 0
}

// ValidateCapacity reports whether adding slotCount more stacks/instances
// keeps inv within its slot limit.
func (inv *Inventory) ValidateCapacity(slotCount int) bool {
	limit, bounded := inv.slotLimit()
	return slotsFit(inv.Size(), slotCount, limit, bounded)
}

// SlotsNeededForItemID reports how many slots count units of templateID
// would take once created in inv: none when a held stackable stack absorbs
// them, one for a new stackable stack, and count for a non-stackable
// template, which takes one slot per unit.
func (inv *Inventory) SlotsNeededForItemID(templateID int32, count int) int {
	tmpl, ok := inv.Templates().Get(templateID)
	if !ok || !tmpl.Stackable {
		return count
	}
	if inv.ItemByTemplateID(templateID) != nil {
		return 0
	}
	return 1
}

// ValidateCapacityByItemID reports whether count new units of templateID
// fit within inv's slot limit.
func (inv *Inventory) ValidateCapacityByItemID(templateID int32, count int) bool {
	return inv.ValidateCapacity(inv.SlotsNeededForItemID(templateID, count))
}

// Delivery handles a live inventory's queued updates and weight changes.
// Inventory itself retains its queue and weight calculation.
type Delivery interface {
	QueueInventoryUpdate(*Inventory)
	UpdateInventoryWeight(*Inventory)
}

// RemovalDelivery is a Delivery that also hears which instances left the
// inventory entirely: destroyed, dropped, or moved to another container.
// A partial count change of a stack that stays is not a removal. It is
// called after the mutation, with no inventory lock held.
type RemovalDelivery interface {
	InventoryItemsRemoved(inv *Inventory, objectIDs []int32)
}

// NewInventory returns an empty inventory owned by ownerID: baseLocation
// for unequipped items (e.g. item.LocationInventory), equipLocation for
// paperdoll items (e.g. item.LocationPaperdoll), resolving templates
// against templates.
func NewInventory(ownerID int32, baseLocation, equipLocation item.Location, templates *item.Table) *Inventory {
	return &Inventory{
		Container:     NewContainer(ownerID, baseLocation, templates),
		equipLocation: equipLocation,
	}
}

// NewPlayerInventory returns an empty player inventory for ownerID.
func NewPlayerInventory(ownerID int32, templates *item.Table) *Inventory {
	inv := NewInventory(ownerID, item.LocationInventory, item.LocationPaperdoll, templates)
	inv.pairsHands = true
	return inv
}

// NewPlayerInventoryWithDelivery returns a player inventory that reports live
// changes through delivery and persists its items through persist.
func NewPlayerInventoryWithDelivery(ownerID int32, templates *item.Table, delivery Delivery, persist item.Persister) *Inventory {
	inv := NewPlayerInventory(ownerID, templates)
	inv.delivery = delivery
	inv.Container.persist = persist
	return inv
}

// RestorePlayerInventory rebuilds a player inventory from persisted item
// rows without queuing client update notifications. Items outside the
// inventory/paperdoll locations are ignored; those belong to warehouses,
// freight, pets, or ground state rather than the carried player inventory.
func RestorePlayerInventory(ownerID int32, templates *item.Table, items []*item.Instance) *Inventory {
	inv := NewPlayerInventory(ownerID, templates)
	inv.Restore(items)
	return inv
}

// RestorePlayerInventoryWithDelivery rebuilds a live player inventory without
// queuing restore notifications; its delivery and persistence dependencies
// are in place before the rows are restored, so restored items carry the
// persister without being re-persisted.
func RestorePlayerInventoryWithDelivery(ownerID int32, templates *item.Table, items []*item.Instance, delivery Delivery, persist item.Persister) *Inventory {
	inv := NewPlayerInventoryWithDelivery(ownerID, templates, delivery, persist)
	inv.Restore(items)
	return inv
}

// NewPetInventory returns an empty pet inventory for ownerID, the object id
// of the pet's collar (not the pet's own world object id, nor its owner's).
// The pet's world object id is transient, while its items must be found
// again after an offline decay of its corpse and across a restart, both of
// which leave only the collar and its pets row. A live pet must use
// NewPetInventoryWithDelivery to send inventory updates.
func NewPetInventory(ownerID int32, templates *item.Table) *Inventory {
	return NewInventory(ownerID, item.LocationPet, item.LocationPetEquip, templates)
}

// NewPetInventoryWithDelivery returns a pet inventory that reports live
// changes through delivery and persists its items through persist.
func NewPetInventoryWithDelivery(ownerID int32, templates *item.Table, delivery Delivery, persist item.Persister) *Inventory {
	inv := NewPetInventory(ownerID, templates)
	inv.delivery = delivery
	inv.Container.persist = persist
	return inv
}

// Add adds inst to the inventory and queues an added/modified notification.
func (inv *Inventory) Add(inst *item.Instance) (result *item.Instance, absorbed bool) {
	result, absorbed = inv.Container.Add(inst)
	if absorbed {
		inv.queueUpdate(result, UpdateModified)
	} else {
		inv.queueUpdate(result, UpdateAdded)
	}
	return result, absorbed
}

// AddNew creates a new instance of templateID and adds it, per
// Container.AddNew, queuing the same notification Add does.
func (inv *Inventory) AddNew(templateID int32, count int, objectID int32) *item.Instance {
	inst, ok := newInstance(inv.Templates(), templateID, count, objectID)
	if !ok {
		return nil
	}
	result, _ := inv.Add(inst)
	return result
}

// Restore replaces inv's current contents with persisted item rows without
// changing their locations and without queuing inventory updates. The only
// writes it schedules are those of a stackable row merged into an earlier
// stack of the same template.
func (inv *Inventory) Restore(items []*item.Instance) {
	inv.Container.mu.Lock()
	defer inv.Container.mu.Unlock()
	inv.mu.Lock()
	defer inv.mu.Unlock()

	clear(inv.Container.items)
	clear(inv.paperdoll[:])
	inv.wornMask = 0
	inv.totalWeight = 0
	inv.updates = nil

	// Every restored row enters the inventory at the same instant, so the
	// stored time never outlives the session that wrote it and the whole
	// restored set ties on it — leaving object id, descending, to order the
	// list the player sees on login.
	restoredAt := nowMillis()

	for _, inst := range items {
		if inst == nil {
			continue
		}
		st := inst.Snapshot()
		switch st.Location {
		case inv.Location(), inv.equipLocation:
		default:
			continue
		}

		// Restoring a row is not a change to persist: the persister is
		// bound only once every row is in place, so neither the owner
		// fix-up nor an equip displacement schedules a redundant write of
		// what was just read.
		inst.EnterContainer(inv.OwnerID(), st.Location, st.LocationData, restoredAt)
		merged := false
		tmpl, _ := inv.Templates().Get(inst.TemplateID)
		if tmpl != nil && tmpl.Stackable {
			for _, held := range inv.Container.items {
				if held.TemplateID == inst.TemplateID {
					// A stack merge is the exception: it changes two rows.
					// Unwritten, the absorbed row survives to be merged in
					// again on every later restore, so both the grown stack
					// and the absorbed row's delete are scheduled here. The
					// grown stack stays bound from here on, so a later row
					// displacing it from its equip slot is written too.
					held.BindPersister(inv.Container.persist)
					inst.BindPersister(inv.Container.persist)
					held.AddCount(st.Count)
					inst.DestroyState()
					merged = true
					break
				}
			}
		}
		if merged {
			continue
		}
		inv.Container.items[inst.ObjectID] = inst

		// totalWeight is deliberately left at 0 here: restoring only fills
		// the item set, and carried weight is computed and reported solely by
		// an explicit UpdateWeight call (as every item-list send makes).
		if st.Location != inv.equipLocation || st.LocationData < 0 || st.LocationData >= item.PaperdollSlots {
			continue
		}
		if tmpl != nil {
			inv.equipItemLocked(inst, tmpl)
		}
	}
	inv.updates = nil
	for _, inst := range inv.Container.items {
		inst.BindPersister(inv.Container.persist)
	}
}

// Remove removes inst from the inventory: unequipping it first if it was
// equipped, then removing it from the underlying container. isDrop
// additionally clears its ownership/location as the final step, once
// unequipping (which itself moves a formerly-equipped instance back to the
// inventory's base location) is already done — otherwise the unequip step
// would clobber the drop reset.
func (inv *Inventory) Remove(inst *item.Instance, isDrop bool) bool {
	if !inv.Container.Remove(inst) {
		return false
	}

	inv.mu.Lock()
	for i, occupant := range inv.paperdoll {
		if occupant == inst {
			// A worn bow or rod leaving takes the left hand with it here
			// too; a caller that has to report that change unequips with
			// UnequipItem before it removes.
			inv.unequipSlotLocked(i)
		}
	}
	inv.mu.Unlock()

	if isDrop {
		st := inst.Snapshot()
		inst.SetOwnerLocation(0, item.LocationVoid, st.LocationData)
	}

	st := inst.Snapshot()
	inv.mu.Lock()
	defer inv.fireDelivery() // registered first, so it runs last, after the unlock
	defer inv.mu.Unlock()
	inv.queueRemovedLocked(st.ObjectID, st.TemplateID, st.Count)
	return true
}

// DestroyItem destroys count units of inst, unequipping and dequeuing it
// first when it's fully consumed and was equipped.
func (inv *Inventory) DestroyItem(inst *item.Instance, count int) *item.Instance {
	if inst == nil {
		return nil
	}
	return destroyItemCore(inst, count, func(inst *item.Instance) bool {
		return inv.Remove(inst, false)
	}, func(inst *item.Instance) {
		inv.queueUpdate(inst, UpdateModified)
	})
}

// DestroyByTemplateID destroys count units of the first instance of
// templateID found, going through the inventory's own DestroyItem — the
// embedded Container's DestroyByTemplateID would call Container.DestroyItem
// directly and bypass the update queue, notifier and unequip bookkeeping
// Inventory.DestroyItem adds.
func (inv *Inventory) DestroyByTemplateID(templateID int32, count int) *item.Instance {
	return inv.DestroyItem(inv.ItemByTemplateID(templateID), count)
}

// DestroyByObjectID destroys count units of the instance identified by
// objectID, per DestroyByTemplateID's reasoning for going through
// Inventory.DestroyItem.
func (inv *Inventory) DestroyByObjectID(objectID int32, count int) *item.Instance {
	return inv.DestroyItem(inv.ItemByObjectID(objectID), count)
}

// DestroyAll destroys every unit of inst, per DestroyByTemplateID's
// reasoning for going through Inventory.DestroyItem.
func (inv *Inventory) DestroyAll(inst *item.Instance) *item.Instance {
	if inst == nil {
		return nil
	}
	return inv.DestroyItem(inst, inst.CountValue())
}

// DestroyAllItems destroys every item instance the inventory holds, clears
// the paperdoll, and queues a removed update for each — the embedded
// Container's version deletes straight from the item map, leaving destroyed
// instances behind in the paperdoll and queuing nothing.
//
// The item-map sweep and the paperdoll clear run in one critical section
// under Container.mu then inv.mu (the order Restore and exchange take), so
// an Add or equip racing this call either lands before it (and is destroyed
// and unequipped with everything else) or blocks until after it (and
// survives as a fresh, consistently equipped item). Clearing the two in
// separate sections would let an item added and equipped between them keep
// its paperdoll location while losing its slot.
func (inv *Inventory) DestroyAllItems() {
	inv.Container.mu.Lock()
	inv.mu.Lock()
	instances := make([]*item.Instance, 0, len(inv.Container.items))
	for objectID, inst := range inv.Container.items {
		delete(inv.Container.items, objectID)
		instances = append(instances, inst)
	}
	clear(inv.paperdoll[:])
	inv.wornMask = 0
	inv.mu.Unlock()
	inv.Container.mu.Unlock()

	for _, inst := range instances {
		st := inst.Snapshot()
		inst.DestroyState()
		inv.mu.Lock()
		inv.queueRemovedLocked(st.ObjectID, st.TemplateID, st.Count)
		inv.mu.Unlock()
	}
	inv.fireDelivery()
}

// SetEnchantLevel changes inst's enchant level and queues a modified
// inventory notification. It returns false when inst is absent from this
// inventory or already has level.
func (inv *Inventory) SetEnchantLevel(inst *item.Instance, level int) bool {
	if inst == nil {
		return false
	}
	if inv.ItemByObjectID(inst.ObjectID) != inst || !inst.SetEnchantLevel(level) {
		return false
	}
	inv.queueUpdate(inst, UpdateModified)
	return true
}

// SetAugmentation gives inst, held in this inventory, aug and queues a
// modified inventory notification. It returns false when inst is absent from
// this inventory or already augmented.
func (inv *Inventory) SetAugmentation(inst *item.Instance, aug item.Augmentation) bool {
	if inst == nil {
		return false
	}
	if inv.ItemByObjectID(inst.ObjectID) != inst || !inst.SetAugmentation(&aug) {
		return false
	}
	inv.queueUpdate(inst, UpdateModified)
	return true
}

// RemoveAugmentation takes the augmentation off inst, held in this
// inventory, and queues a modified inventory notification. It returns false
// when inst is absent from this inventory or carries none.
func (inv *Inventory) RemoveAugmentation(inst *item.Instance) bool {
	if inst == nil || inv.ItemByObjectID(inst.ObjectID) != inst {
		return false
	}
	if _, ok := inst.RemoveAugmentation(); !ok {
		return false
	}
	inv.queueUpdate(inst, UpdateModified)
	return true
}

// DropItem removes count units of the instance identified by objectID from
// the inventory for dropping to the ground. When the held stack is bigger
// than count, the existing stack is decremented in place and a brand new
// instance carrying just the dropped count is returned instead (using
// newObjectID as its pre-allocated world id) — matching how a partial drop
// splits off a fresh stack rather than reusing the original one's identity.
// Otherwise the whole instance is removed from the inventory and returned
// as-is (newObjectID unused). It returns nil if objectID isn't held.
func (inv *Inventory) DropItem(objectID int32, count int, newObjectID int32) *item.Instance {
	inst := inv.ItemByObjectID(objectID)
	if inst == nil {
		return nil
	}

	st := inst.Snapshot()
	if st.Count > count {
		if _, ok := inst.ReduceCount(count); !ok {
			return nil
		}
		inv.queueUpdate(inst, UpdateModified)

		tmpl, _ := inv.Templates().Get(st.TemplateID)
		var manaLeft int
		if tmpl != nil {
			manaLeft = tmpl.InitialManaLeft()
		} else {
			manaLeft = -1
		}
		return &item.Instance{ObjectID: newObjectID, TemplateID: st.TemplateID, Count: count, ManaLeft: manaLeft}
	}

	if !inv.Remove(inst, true) {
		return nil
	}
	return inst
}

// TransferItem moves count units from inv to target and queues inventory
// updates on inv for the source-side change. A target inventory's Add path
// queues its own update.
func (inv *Inventory) TransferItem(objectID int32, count int, target Receiver, newObjectID int32) (result *item.Instance, freedObjectID int32, freed bool) {
	if target == nil || count <= 0 {
		return nil, 0, false
	}
	source := inv.ItemByObjectID(objectID)
	if source == nil {
		return nil, 0, false
	}
	st := source.Snapshot()
	templateID := st.TemplateID
	sourceCount := st.Count
	movedCount := count
	if movedCount > sourceCount {
		movedCount = sourceCount
	}

	result, freedObjectID, freed = inv.Container.Transfer(objectID, count, target, newObjectID)
	if result == nil {
		return nil, 0, false
	}
	if remaining := inv.ItemByObjectID(objectID); remaining != nil {
		inv.queueUpdate(remaining, UpdateModified)
	} else {
		inv.mu.Lock()
		inv.queueRemovedLocked(objectID, templateID, movedCount)
		inv.mu.Unlock()
		inv.fireDelivery()
	}
	return result, freedObjectID, freed
}

// ItemAt returns the instance equipped at paperdoll position slot, or nil.
func (inv *Inventory) ItemAt(slot int) *item.Instance {
	if slot < 0 || slot >= item.PaperdollSlots {
		return nil
	}
	inv.mu.Lock()
	defer inv.mu.Unlock()
	return inv.paperdoll[slot]
}

// PaperdollItems returns every currently equipped instance.
func (inv *Inventory) PaperdollItems() []*item.Instance {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	var out []*item.Instance
	for _, occupant := range inv.paperdoll {
		if occupant != nil {
			out = append(out, occupant)
		}
	}
	return out
}

// IsWearingType reports whether any currently equipped weapon or armor
// contributes mask to the inventory's worn-type mask (see item.Template.Mask).
func (inv *Inventory) IsWearingType(mask int32) bool {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	return inv.wornMask&mask != 0
}

// SetPaperdollItem places inst (its template is tmpl) at paperdoll
// position slot, replacing and returning whatever instance occupied it.
// Passing a nil inst clears the slot. Equipping/unequipping updates the
// occupant's own Location/LocationData and the inventory's worn-type mask;
// a two-piece chest/legs pairing only contributes its mask bit when both
// pieces share the same armor type.
func (inv *Inventory) SetPaperdollItem(slot int, inst *item.Instance, tmpl *item.Template) *item.Instance {
	inv.mu.Lock()
	defer inv.fireDelivery() // registered first, so it runs last, after the unlock
	defer inv.mu.Unlock()
	return inv.setPaperdollItemLocked(slot, inst, tmpl)
}

func (inv *Inventory) setPaperdollItemLocked(slot int, inst *item.Instance, tmpl *item.Template) *item.Instance {
	old := inv.paperdoll[slot]
	if old == inst {
		return old
	}

	if old != nil {
		inv.paperdoll[slot] = nil
		old.SetLocation(inv.Location(), 0)
		if oldTmpl, ok := inv.Templates().Get(old.TemplateID); ok {
			inv.wornMask &^= oldTmpl.Mask()
		}
		inv.queueUpdateLocked(old, UpdateModified)
	}

	if inst != nil {
		inv.paperdoll[slot] = inst
		inst.SetLocation(inv.equipLocation, slot)
		inv.queueUpdateLocked(inst, UpdateModified)

		switch {
		case tmpl != nil && tmpl.Slot == item.SlotChest:
			if legs := inv.paperdoll[Legs]; legs != nil {
				if legsTmpl, ok := inv.Templates().Get(legs.TemplateID); ok && legsTmpl.Mask() == tmpl.Mask() {
					inv.wornMask |= tmpl.Mask()
				}
			}
		case tmpl != nil && tmpl.Slot == item.SlotLegs:
			if chest := inv.paperdoll[Chest]; chest != nil {
				if chestTmpl, ok := inv.Templates().Get(chest.TemplateID); ok && chestTmpl.Mask() == tmpl.Mask() {
					inv.wornMask |= tmpl.Mask()
				}
			}
		case tmpl != nil:
			inv.wornMask |= tmpl.Mask()
		}
	}

	return old
}

// EquipItem places inst (its template is tmpl) into whichever paperdoll
// position(s) its body slot maps to, resolving the slot-sharing and
// mutual-exclusion rules the client expects (a two-handed weapon clears
// the off hand except for a bow/arrow or fishing rod/lure pairing, a full
// set of formal wear clears every other equip slot, and so on). It returns
// every instance whose equip state changed as a result (the newly equipped
// item plus any implicitly unequipped ones).
//
// A bow looks its arrows up among the held items, so the equip holds
// Container.mu for reading ahead of mu, the order Restore takes them in.
func (inv *Inventory) EquipItem(inst *item.Instance, tmpl *item.Template) []*item.Instance {
	inv.Container.mu.RLock()
	inv.mu.Lock()
	defer inv.fireDelivery() // registered first, so it runs last, after the unlocks
	defer inv.Container.mu.RUnlock()
	defer inv.mu.Unlock()
	return inv.equipItemLocked(inst, tmpl)
}

// EquipPlayerItem is EquipItem for a player's paperdoll, which first applies
// the worn formal wear rule: while the chest slot holds a full-body dress, a
// weapon, shield or other hand item takes the dress off before going on, and
// legs, feet, gloves or a helmet cannot go on at all (refused reports that,
// with nothing changed). Its altered list includes the removed dress.
func (inv *Inventory) EquipPlayerItem(inst *item.Instance, tmpl *item.Template) (altered []*item.Instance, refused bool) {
	inv.Container.mu.RLock()
	inv.mu.Lock()
	defer inv.fireDelivery() // registered first, so it runs last, after the unlocks
	defer inv.Container.mu.RUnlock()
	defer inv.mu.Unlock()

	if chest := inv.paperdoll[Chest]; chest != nil {
		if chestTmpl, ok := inv.Templates().Get(chest.TemplateID); ok && chestTmpl.Slot == item.SlotAllDress {
			switch tmpl.Slot {
			case item.SlotLRHand, item.SlotLHand, item.SlotRHand:
				if old := inv.setPaperdollItemLocked(Chest, nil, nil); old != nil {
					altered = append(altered, old)
				}
			case item.SlotLegs, item.SlotFeet, item.SlotGloves, item.SlotHead:
				return nil, true
			}
		}
	}
	return append(altered, inv.equipItemLocked(inst, tmpl)...), false
}

// equipItemLocked is EquipItem's body. The caller holds Container.mu (for
// reading at least) and mu. Each instance whose equip state changed is
// reported once, in the order the paperdoll changed it: an item the bow or
// rod rule moved in or out of the left hand comes just before the right-hand
// item that moved it.
func (inv *Inventory) equipItemLocked(inst *item.Instance, tmpl *item.Template) []*item.Instance {
	var altered []*item.Instance
	record := func(changed ...*item.Instance) {
		for _, c := range changed {
			if !slices.Contains(altered, c) {
				altered = append(altered, c)
			}
		}
	}
	set := func(slot int) {
		if old := inv.setPaperdollItemLocked(slot, inst, tmpl); old != nil && old != inst {
			record(inv.releaseOffHandLocked(slot, old)...)
			record(old)
		}
		record(inv.pullOffHandLocked(slot, inst, tmpl)...)
		record(inst)
	}
	clearSlot := func(slot int) {
		if old := inv.setPaperdollItemLocked(slot, nil, nil); old != nil {
			record(inv.releaseOffHandLocked(slot, old)...)
			record(old)
		}
	}
	occupantTemplate := func(slot int) *item.Template {
		occ := inv.paperdoll[slot]
		if occ == nil {
			return nil
		}
		t, _ := inv.Templates().Get(occ.TemplateID)
		return t
	}

	switch tmpl.Slot {
	case item.SlotLRHand:
		clearSlot(LHand)
		set(RHand)

	case item.SlotLHand:
		if rhTmpl := occupantTemplate(RHand); rhTmpl != nil && rhTmpl.Slot == item.SlotLRHand {
			pairedBowArrow := rhTmpl.Weapon != nil && rhTmpl.Weapon.Type == item.WeaponBow &&
				tmpl.EtcItem != nil && tmpl.EtcItem.Type == item.EtcItemArrow
			pairedRodLure := rhTmpl.Weapon != nil && rhTmpl.Weapon.Type == item.WeaponFishingRod &&
				tmpl.EtcItem != nil && tmpl.EtcItem.Type == item.EtcItemLure
			if !pairedBowArrow && !pairedRodLure {
				clearSlot(RHand)
			}
		}
		set(LHand)

	case item.SlotRHand:
		set(RHand)

	case item.SlotLEar, item.SlotREar, item.SlotLREar:
		inv.equipPaired(tmpl, LEar, REar, set)

	case item.SlotLFinger, item.SlotRFinger, item.SlotLRFinger:
		inv.equipPaired(tmpl, LFinger, RFinger, set)

	case item.SlotNeck:
		set(Neck)

	case item.SlotFullArmor:
		clearSlot(Legs)
		set(Chest)

	case item.SlotChest:
		set(Chest)

	case item.SlotLegs:
		if chestTmpl := occupantTemplate(Chest); chestTmpl != nil && chestTmpl.Slot == item.SlotFullArmor {
			clearSlot(Chest)
		}
		set(Legs)

	case item.SlotFeet:
		set(Feet)

	case item.SlotGloves:
		set(Gloves)

	case item.SlotHead:
		set(Head)

	case item.SlotFace:
		if hairTmpl := occupantTemplate(Hair); hairTmpl != nil && hairTmpl.Slot == item.SlotHairAll {
			clearSlot(Hair)
		}
		set(Face)

	case item.SlotHair:
		if faceTmpl := occupantTemplate(Face); faceTmpl != nil && faceTmpl.Slot == item.SlotHairAll {
			clearSlot(Face)
		}
		set(Hair)

	case item.SlotHairAll:
		clearSlot(Face)
		set(Hair)

	case item.SlotUnderwear:
		set(Under)

	case item.SlotBack:
		set(Cloak)

	case item.SlotAllDress:
		clearSlot(Legs)
		clearSlot(LHand)
		clearSlot(RHand)
		clearSlot(Head)
		clearSlot(Feet)
		clearSlot(Gloves)
		set(Chest)

	default:
		// Unknown body slot: the shipped data never produces one, so this
		// is a no-op rather than a hard error.
	}

	return altered
}

func (inv *Inventory) equipPaired(tmpl *item.Template, slotA, slotB int, set func(int)) {
	switch {
	case inv.paperdoll[slotA] == nil:
		set(slotA)
	case inv.paperdoll[slotB] == nil:
		set(slotB)
	default:
		aID, bID := inv.paperdoll[slotA].TemplateID, inv.paperdoll[slotB].TemplateID
		switch tmpl.ID {
		case bID:
			set(slotA)
		case aID:
			set(slotB)
		default:
			set(slotA)
		}
	}
}

// UnequipSlot clears whatever instance occupies paperdoll position slot
// and returns it, or nil if the slot was already empty. Every equipped
// instance already records which paperdoll position it occupies
// (Instance.LocationData), so resolving that position back through the
// item's body-slot bits first is unnecessary — it always round-trips to the
// same position. The left hand a bow or rod held is cleared with it; use
// UnequipItem to learn about that change too.
func (inv *Inventory) UnequipSlot(slot int) *item.Instance {
	inv.mu.Lock()
	defer inv.fireDelivery() // registered first, so it runs last, after the unlock
	defer inv.mu.Unlock()
	changed := inv.unequipSlotLocked(slot)
	if len(changed) == 0 {
		return nil
	}
	return changed[len(changed)-1]
}

// UnequipItem takes inst off the paperdoll when it is still worn and
// returns every instance whose equip state changed, in the order the
// paperdoll changed them: a bow's or rod's arrows or lure come before the
// bow or rod itself. It returns nil when inst is not worn.
func (inv *Inventory) UnequipItem(inst *item.Instance) []*item.Instance {
	if inst == nil {
		return nil
	}
	inv.mu.Lock()
	defer inv.fireDelivery() // registered first, so it runs last, after the unlock
	defer inv.mu.Unlock()
	for slot, occupant := range inv.paperdoll {
		if occupant == inst {
			return inv.unequipSlotLocked(slot)
		}
	}
	return nil
}

// unequipSlotLocked clears paperdoll position slot and returns what it
// changed, the left hand a bow or rod held first. The caller holds mu.
func (inv *Inventory) unequipSlotLocked(slot int) []*item.Instance {
	if slot < 0 || slot >= item.PaperdollSlots {
		return nil
	}
	old := inv.setPaperdollItemLocked(slot, nil, nil)
	if old == nil {
		return nil
	}
	return append(inv.releaseOffHandLocked(slot, old), old)
}

// releaseOffHandLocked applies the first half of the player's bow/rod rule
// after old left paperdoll position slot: a bow or fishing rod leaving the
// right hand clears the left hand. It returns the instance it took off, if
// any. The caller holds mu.
func (inv *Inventory) releaseOffHandLocked(slot int, old *item.Instance) []*item.Instance {
	if !inv.pairsHands || slot != RHand || old == nil {
		return nil
	}
	tmpl, ok := inv.Templates().Get(old.TemplateID)
	if !ok || tmpl.Weapon == nil || (tmpl.Weapon.Type != item.WeaponBow && tmpl.Weapon.Type != item.WeaponFishingRod) {
		return nil
	}
	if off := inv.setPaperdollItemLocked(LHand, nil, nil); off != nil {
		return []*item.Instance{off}
	}
	return nil
}

// pullOffHandLocked applies the second half of the player's bow/rod rule
// after inst (its template is tmpl) went into paperdoll position slot: a
// bow in the right hand takes the held arrows of its grade into the left
// hand. It returns what it moved: the arrows, and whatever they displaced.
// The caller holds Container.mu (for reading at least) and mu.
func (inv *Inventory) pullOffHandLocked(slot int, inst *item.Instance, tmpl *item.Template) []*item.Instance {
	if !inv.pairsHands || slot != RHand || inst == nil || tmpl == nil || tmpl.Weapon == nil || tmpl.Weapon.Type != item.WeaponBow {
		return nil
	}
	arrowID, ok := arrowIDForCrystal(tmpl.Crystal)
	if !ok {
		return nil
	}
	arrows := inv.Container.itemByTemplateIDLocked(arrowID)
	if arrows == nil {
		return nil
	}
	arrowTmpl, ok := inv.Templates().Get(arrowID)
	if !ok {
		return nil
	}
	var moved []*item.Instance
	if off := inv.setPaperdollItemLocked(LHand, arrows, arrowTmpl); off != nil && off != arrows {
		moved = append(moved, off)
	}
	return append(moved, arrows)
}

// ClearWornSlot drops paperdoll position slot's occupant and its wornMask
// contribution when it still equals inst, without touching inst's own
// Location/LocationData or queuing an inventory update for it. Use this
// instead of UnequipSlot when inst has already been moved to a different
// container (e.g. by a completed transfer) and only this inventory's own
// equip bookkeeping needs to catch up — UnequipSlot's SetLocation(inv.
// Location(), 0) would otherwise stomp the location the transfer already
// assigned. Reports whether it actually cleared anything.
func (inv *Inventory) ClearWornSlot(slot int, inst *item.Instance) bool {
	if slot < 0 || slot >= item.PaperdollSlots || inst == nil {
		return false
	}
	inv.mu.Lock()
	defer inv.fireDelivery() // registered first, so it runs last, after the unlock
	defer inv.mu.Unlock()
	if inv.paperdoll[slot] != inst {
		return false
	}
	inv.paperdoll[slot] = nil
	if tmpl, ok := inv.Templates().Get(inst.TemplateID); ok {
		inv.wornMask &^= tmpl.Mask()
	}
	return true
}

// UpdateWeight recomputes the inventory's total carried weight and reports
// whether it changed.
func (inv *Inventory) UpdateWeight() bool {
	weight := inv.carriedWeight()

	inv.mu.Lock()
	if inv.totalWeight == weight {
		inv.mu.Unlock()
		return false
	}
	inv.totalWeight = weight
	delivery := inv.delivery
	inv.mu.Unlock()
	if delivery != nil {
		delivery.UpdateInventoryWeight(inv)
	}
	return true
}

// TotalWeight returns the inventory's last-computed total carried weight.
func (inv *Inventory) TotalWeight() int {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	return inv.totalWeight
}

// carriedWeight sums the weight of every item inv holds now.
func (inv *Inventory) carriedWeight() int {
	inv.Container.mu.RLock()
	defer inv.Container.mu.RUnlock()
	return inv.carriedWeightLocked()
}

// carriedWeightLocked is carriedWeight for a caller holding Container.mu.
func (inv *Inventory) carriedWeightLocked() int {
	weight := 0
	for _, inst := range inv.items {
		if tmpl, ok := inv.templates.Get(inst.TemplateID); ok {
			weight += int(tmpl.Weight) * inst.Snapshot().Count
		}
	}
	return weight
}

// ValidateWeight reports whether adding weight more keeps the inventory
// within its weight limit: its Limiter's, or WeightLimit, where 0 means
// unlimited. It weighs what inv holds now rather than the last weight
// UpdateWeight delivered, which trails item changes until the next update.
func (inv *Inventory) ValidateWeight(weight int) bool {
	limit, bounded := inv.weightLimit()
	if !bounded {
		return true
	}
	return inv.carriedWeight()+weight <= limit
}

// SlotsNeededFor reports how many capacity slots adding inst (of template
// tmpl) would consume: 0 when it merges into an existing stack or is a
// herb (herbs are used instantly and never actually occupy a slot), 1
// otherwise.
func (inv *Inventory) SlotsNeededFor(inst *item.Instance, tmpl *item.Template) int {
	if tmpl.Stackable && inv.ItemByTemplateID(inst.TemplateID) != nil {
		return 0
	}
	if tmpl.EtcItem != nil && tmpl.EtcItem.Type == item.EtcItemHerb {
		return 0
	}
	return 1
}

func arrowIDForCrystal(crystal item.CrystalType) (int32, bool) {
	switch crystal {
	case item.CrystalNone:
		return 17, true
	case item.CrystalD:
		return 1341, true
	case item.CrystalC:
		return 1342, true
	case item.CrystalB:
		return 1343, true
	case item.CrystalA:
		return 1344, true
	case item.CrystalS:
		return 1345, true
	default:
		return 0, false
	}
}

// FindArrowForBow returns the instance of the arrow matching bowCrystal
// currently held, or nil if the inventory holds none.
func (inv *Inventory) FindArrowForBow(bowCrystal item.CrystalType) *item.Instance {
	arrowID, ok := arrowIDForCrystal(bowCrystal)
	if !ok {
		return nil
	}
	return inv.ItemByTemplateID(arrowID)
}

// DrainUpdates returns every pending inventory-change notification queued
// since the last DrainUpdates call, then clears the queue. Delivering
// these as an actual client packet is the network layer's job.
func (inv *Inventory) DrainUpdates() []Update {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	out := inv.updates
	inv.updates = nil
	return out
}

// BuildAndDrainUpdates snapshots inv's items and drains the pending-update
// queue in one critical section, then passes the snapshot to build with no
// lock held. Used where a full-list snapshot must double as the update
// checkpoint, e.g. sending PetItemList on discover.
//
// Taking the snapshot and the drain together keeps a concurrent change (the
// auto-feed ticker, a trade partner's give) from landing between them and
// being lost: it is either inside the snapshot and drained, or queued after
// it and delivered by the next tick. build runs unlocked, so it may call back
// into inv. If build fails (e.g. a persisted item whose template never
// loaded), the drained updates go back ahead of any queued since, so nothing
// is thrown away unsent.
func (inv *Inventory) BuildAndDrainUpdates(build func(items []*item.Instance) error) error {
	inv.Container.mu.RLock()
	inv.mu.Lock()
	items := inv.itemsLocked()
	drained := inv.updates
	inv.updates = nil
	inv.mu.Unlock()
	inv.Container.mu.RUnlock()

	if err := build(items); err != nil {
		inv.requeueUpdates(drained)
		return err
	}
	return nil
}

// requeueUpdates puts drained back at the head of the queue and folds the
// updates queued since into it with the usual coalescing.
func (inv *Inventory) requeueUpdates(drained []Update) {
	if len(drained) == 0 {
		return
	}
	inv.mu.Lock()
	defer inv.fireDelivery() // registered first, so it runs last, after the unlock
	defer inv.mu.Unlock()
	newer := inv.updates
	inv.updates = drained
	for _, u := range newer {
		inv.queueUpdateRecordLocked(u.ObjectID, u.TemplateID, u.Count, u.State)
	}
}

// HasUpdates reports whether any inventory-change notifications are queued.
func (inv *Inventory) HasUpdates() bool {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	return len(inv.updates) != 0
}

func (inv *Inventory) queueUpdate(inst *item.Instance, state UpdateState) {
	inv.mu.Lock()
	defer inv.fireDelivery() // registered first, so it runs last, after the unlock
	defer inv.mu.Unlock()
	inv.queueUpdateLocked(inst, state)
}

func (inv *Inventory) queueUpdateLocked(inst *item.Instance, state UpdateState) {
	if inst == nil {
		return
	}
	st := inst.Snapshot()
	inv.queueUpdateRecordLocked(st.ObjectID, st.TemplateID, st.Count, state)
}

func (inv *Inventory) queueUpdateRecordLocked(objectID, templateID int32, count int, state UpdateState) {
	// Coalesce a repeated update for the same stackable instance and state
	// (e.g. several count changes in a row) into the latest count instead of
	// letting the queue grow unbounded; any other update is appended as its
	// own entry.
	tmpl, _ := inv.Templates().Get(templateID)
	if tmpl != nil && tmpl.Stackable {
		for i, u := range inv.updates {
			if u.ObjectID == objectID && u.State == state {
				inv.updates[i].Count = count
				return
			}
		}
	}
	inv.updates = append(inv.updates, Update{ObjectID: objectID, TemplateID: templateID, Count: count, State: state})
}

// queueRemovedLocked queues the removed update of an instance that left the
// inventory and records its object id for the next fireDelivery.
func (inv *Inventory) queueRemovedLocked(objectID, templateID int32, count int) {
	inv.queueUpdateRecordLocked(objectID, templateID, count, UpdateRemoved)
	inv.removed = append(inv.removed, objectID)
}

// fireDelivery reports a queued update after a mutation, then the instances
// that left since the last call. It reads delivery under the lock and calls
// it outside so the delivery can inspect inv safely.
func (inv *Inventory) fireDelivery() {
	inv.mu.Lock()
	delivery := inv.delivery
	pending := len(inv.updates) > 0
	removed := inv.removed
	inv.removed = nil
	inv.mu.Unlock()
	if delivery == nil {
		return
	}
	if pending {
		delivery.QueueInventoryUpdate(inv)
	}
	if rd, ok := delivery.(RemovalDelivery); ok && len(removed) > 0 {
		rd.InventoryItemsRemoved(inv, removed)
	}
}
