package itemcontainer

import (
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

// Update states.
const (
	UpdateAdded UpdateState = iota
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
// mu guards paperdoll, wornMask, totalWeight and updates. It does not make
// the *item.Instance values themselves thread-safe; those follow Container's
// owner-serialized access rule.
type Inventory struct {
	*Container

	equipLocation item.Location

	WeightLimit int

	mu          sync.Mutex
	paperdoll   [item.PaperdollSlots]*item.Instance
	wornMask    int32
	totalWeight int
	updates     []Update
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
	return NewInventory(ownerID, item.LocationInventory, item.LocationPaperdoll, templates)
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

// NewPetInventory returns an empty pet inventory for ownerID (the pet's
// own world object id, not its owner's).
func NewPetInventory(ownerID int32, templates *item.Table) *Inventory {
	return NewInventory(ownerID, item.LocationPet, item.LocationPetEquip, templates)
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
	tmpl, ok := inv.Templates().Get(templateID)
	if !ok {
		return nil
	}
	if count < 1 {
		count = 1
	}
	if !tmpl.Stackable {
		count = 1
	}
	inst := &item.Instance{ObjectID: objectID, TemplateID: templateID, Count: count, ManaLeft: tmpl.InitialManaLeft()}
	result, _ := inv.Add(inst)
	return result
}

// Restore replaces inv's current contents with persisted item rows without
// changing their locations and without queuing inventory updates.
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

	for _, inst := range items {
		if inst == nil {
			continue
		}
		switch inst.Location {
		case inv.Location(), inv.equipLocation:
		default:
			continue
		}

		inst.OwnerID = inv.OwnerID()
		inv.Container.items[inst.ObjectID] = inst

		tmpl, _ := inv.Templates().Get(inst.TemplateID)
		if tmpl != nil {
			inv.totalWeight += int(tmpl.Weight) * inst.Count
		}
		if inst.Location != inv.equipLocation || inst.LocationData < 0 || inst.LocationData >= item.PaperdollSlots {
			continue
		}
		inv.paperdoll[inst.LocationData] = inst
		if tmpl != nil {
			inv.wornMask |= tmpl.Mask()
		}
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
			inv.unequipSlotLocked(i)
		}
	}
	inv.mu.Unlock()

	if isDrop {
		inst.OwnerID = 0
		inst.Location = item.LocationVoid
	}

	inv.queueUpdate(inst, UpdateRemoved)
	return true
}

// DestroyItem destroys count units of inst, unequipping and dequeuing it
// first when it's fully consumed and was equipped.
func (inv *Inventory) DestroyItem(inst *item.Instance, count int) *item.Instance {
	if inst == nil {
		return nil
	}
	if inst.Count > count {
		inst.Count -= count
		inv.queueUpdate(inst, UpdateModified)
		return inst
	}
	if inst.Count < count {
		return nil
	}
	if !inv.Remove(inst, false) {
		return nil
	}
	inst.Count = 0
	inst.OwnerID = 0
	inst.Location = item.LocationVoid
	return inst
}

// SetEnchantLevel changes inst's enchant level and queues a modified
// inventory notification. It returns false when inst is absent from this
// inventory or already has level.
func (inv *Inventory) SetEnchantLevel(inst *item.Instance, level int) bool {
	if inst == nil {
		return false
	}
	if inv.ItemByObjectID(inst.ObjectID) != inst || inst.EnchantLevel == level {
		return false
	}
	inst.EnchantLevel = level
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

	if inst.Count > count {
		inst.Count -= count
		inv.queueUpdate(inst, UpdateModified)

		tmpl, _ := inv.Templates().Get(inst.TemplateID)
		var manaLeft int
		if tmpl != nil {
			manaLeft = tmpl.InitialManaLeft()
		} else {
			manaLeft = -1
		}
		return &item.Instance{ObjectID: newObjectID, TemplateID: inst.TemplateID, Count: count, ManaLeft: manaLeft}
	}

	if !inv.Remove(inst, true) {
		return nil
	}
	return inst
}

// TransferItem moves count units from inv to target and queues inventory
// updates on inv for the source-side change. The target inventory's Add path
// queues its own update.
func (inv *Inventory) TransferItem(objectID int32, count int, target *Inventory, newObjectID int32) (result *item.Instance, freedObjectID int32, freed bool) {
	if target == nil || count <= 0 {
		return nil, 0, false
	}
	source := inv.ItemByObjectID(objectID)
	if source == nil {
		return nil, 0, false
	}
	templateID := source.TemplateID
	sourceCount := source.Count
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
		inv.queueUpdateRecord(objectID, templateID, movedCount, UpdateRemoved)
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
		old.Location = inv.Location()
		old.LocationData = 0
		if oldTmpl, ok := inv.Templates().Get(old.TemplateID); ok {
			inv.wornMask &^= oldTmpl.Mask()
		}
		inv.queueUpdateLocked(old, UpdateModified)
	}

	if inst != nil {
		inv.paperdoll[slot] = inst
		inst.Location = inv.equipLocation
		inst.LocationData = slot
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
func (inv *Inventory) EquipItem(inst *item.Instance, tmpl *item.Template) []*item.Instance {
	inv.mu.Lock()
	defer inv.mu.Unlock()

	var altered []*item.Instance
	set := func(slot int) {
		if old := inv.setPaperdollItemLocked(slot, inst, tmpl); old != nil {
			altered = append(altered, old)
		}
		altered = append(altered, inst)
	}
	clear := func(slot int) {
		if old := inv.setPaperdollItemLocked(slot, nil, nil); old != nil {
			altered = append(altered, old)
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
		clear(LHand)
		set(RHand)

	case item.SlotLHand:
		if rhTmpl := occupantTemplate(RHand); rhTmpl != nil && rhTmpl.Slot == item.SlotLRHand {
			pairedBowArrow := rhTmpl.Weapon != nil && rhTmpl.Weapon.Type == item.WeaponBow &&
				tmpl.EtcItem != nil && tmpl.EtcItem.Type == item.EtcItemArrow
			pairedRodLure := rhTmpl.Weapon != nil && rhTmpl.Weapon.Type == item.WeaponFishingRod &&
				tmpl.EtcItem != nil && tmpl.EtcItem.Type == item.EtcItemLure
			if !pairedBowArrow && !pairedRodLure {
				clear(RHand)
			}
		}
		set(LHand)

	case item.SlotRHand:
		set(RHand)

	case item.SlotLEar, item.SlotREar, item.SlotLREar:
		switch {
		case inv.paperdoll[LEar] == nil:
			set(LEar)
		case inv.paperdoll[REar] == nil:
			set(REar)
		default:
			rearID, learID := int32(0), int32(0)
			if inv.paperdoll[REar] != nil {
				rearID = inv.paperdoll[REar].TemplateID
			}
			if inv.paperdoll[LEar] != nil {
				learID = inv.paperdoll[LEar].TemplateID
			}
			switch tmpl.ID {
			case rearID:
				set(LEar)
			case learID:
				set(REar)
			default:
				set(LEar)
			}
		}

	case item.SlotLFinger, item.SlotRFinger, item.SlotLRFinger:
		switch {
		case inv.paperdoll[LFinger] == nil:
			set(LFinger)
		case inv.paperdoll[RFinger] == nil:
			set(RFinger)
		default:
			rfingerID, lfingerID := int32(0), int32(0)
			if inv.paperdoll[RFinger] != nil {
				rfingerID = inv.paperdoll[RFinger].TemplateID
			}
			if inv.paperdoll[LFinger] != nil {
				lfingerID = inv.paperdoll[LFinger].TemplateID
			}
			switch tmpl.ID {
			case rfingerID:
				set(LFinger)
			case lfingerID:
				set(RFinger)
			default:
				set(LFinger)
			}
		}

	case item.SlotNeck:
		set(Neck)

	case item.SlotFullArmor:
		clear(Legs)
		set(Chest)

	case item.SlotChest:
		set(Chest)

	case item.SlotLegs:
		if chestTmpl := occupantTemplate(Chest); chestTmpl != nil && chestTmpl.Slot == item.SlotFullArmor {
			clear(Chest)
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
			clear(Hair)
		}
		set(Face)

	case item.SlotHair:
		if faceTmpl := occupantTemplate(Face); faceTmpl != nil && faceTmpl.Slot == item.SlotHairAll {
			clear(Face)
		}
		set(Hair)

	case item.SlotHairAll:
		clear(Face)
		set(Hair)

	case item.SlotUnderwear:
		set(Under)

	case item.SlotBack:
		set(Cloak)

	case item.SlotAllDress:
		clear(Legs)
		clear(LHand)
		clear(RHand)
		clear(Head)
		clear(Feet)
		clear(Gloves)
		set(Chest)

	default:
		// Unknown body slot: the shipped data never produces one, so this
		// is a no-op rather than a hard error.
	}

	return altered
}

// UnequipSlot clears whatever instance occupies paperdoll position slot
// and returns it, or nil if the slot was already empty. Unlike the Java
// reference's separate "unequip by body slot" path, this is the only
// unequip primitive: every equipped instance already records which
// paperdoll position it occupies (Instance.LocationData), so resolving
// that position back through the item's body-slot bits first is
// unnecessary — it always round-trips to the same position.
func (inv *Inventory) UnequipSlot(slot int) *item.Instance {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	return inv.unequipSlotLocked(slot)
}

func (inv *Inventory) unequipSlotLocked(slot int) *item.Instance {
	if slot < 0 || slot >= item.PaperdollSlots {
		return nil
	}
	return inv.setPaperdollItemLocked(slot, nil, nil)
}

// UpdateWeight recomputes the inventory's total carried weight and reports
// whether it changed.
func (inv *Inventory) UpdateWeight() bool {
	weight := 0
	for _, inst := range inv.Items() {
		if tmpl, ok := inv.Templates().Get(inst.TemplateID); ok {
			weight += int(tmpl.Weight) * inst.Count
		}
	}

	inv.mu.Lock()
	defer inv.mu.Unlock()
	if inv.totalWeight == weight {
		return false
	}
	inv.totalWeight = weight
	return true
}

// TotalWeight returns the inventory's last-computed total carried weight.
func (inv *Inventory) TotalWeight() int {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	return inv.totalWeight
}

// ValidateWeight reports whether adding weight more keeps the inventory
// within WeightLimit. A WeightLimit of 0 means unlimited.
func (inv *Inventory) ValidateWeight(weight int) bool {
	if inv.WeightLimit <= 0 {
		return true
	}
	return inv.TotalWeight()+weight <= inv.WeightLimit
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

// arrowForCrystal maps a bow's crystal grade to the matching arrow item id.
var arrowForCrystal = map[item.CrystalType]int32{
	item.CrystalNone: 17,   // wooden arrow
	item.CrystalD:    1341, // bone arrow
	item.CrystalC:    1342, // fine steel arrow
	item.CrystalB:    1343, // silver arrow
	item.CrystalA:    1344, // mithril arrow
	item.CrystalS:    1345, // shining arrow
}

// FindArrowForBow returns the instance of the arrow matching bowCrystal
// currently held, or nil if the inventory holds none.
func (inv *Inventory) FindArrowForBow(bowCrystal item.CrystalType) *item.Instance {
	arrowID, ok := arrowForCrystal[bowCrystal]
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

// HasUpdates reports whether any inventory-change notifications are queued.
func (inv *Inventory) HasUpdates() bool {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	return len(inv.updates) != 0
}

func (inv *Inventory) queueUpdate(inst *item.Instance, state UpdateState) {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	inv.queueUpdateLocked(inst, state)
}

func (inv *Inventory) queueUpdateLocked(inst *item.Instance, state UpdateState) {
	if inst == nil {
		return
	}
	inv.queueUpdateRecordLocked(inst.ObjectID, inst.TemplateID, inst.Count, state)
}

func (inv *Inventory) queueUpdateRecord(objectID, templateID int32, count int, state UpdateState) {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	inv.queueUpdateRecordLocked(objectID, templateID, count, state)
}

func (inv *Inventory) queueUpdateRecordLocked(objectID, templateID int32, count int, state UpdateState) {
	// Coalesce a repeated update for the same instance and state (e.g.
	// several count changes in a row) into the latest count, matching the
	// Java reference's dedup rule, instead of letting the queue grow
	// unbounded.
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
