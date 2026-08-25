package item

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

// Instance is one persisted item: a template plus the owner-specific state
// recorded in the items table (how many, where it sits, its enchant level).
//
// ObjectID and TemplateID are immutable after construction. The embedded live
// state is mutated by inventories, container transfers, background shadow-item
// decay, and lazy persistence; access that state through the methods below
// once an instance is visible outside construction/restore code.
type Instance struct {
	mu unsafe.Pointer

	ObjectID   int32
	TemplateID int32
	OwnerID    int32

	Count        int
	EnchantLevel int

	Location     Location
	LocationData int

	CustomType1, CustomType2 int
	ManaLeft                 int
	Time                     int64

	// ShotsMask tracks which ShotKinds are currently charged on this
	// instance (soulshot/spiritshot bonus applied to the next hit). It is
	// transient, in-memory-only state: charges don't survive a restart.
	ShotsMask int32

	// Augmentation is the life-stone bonus applied to this instance, or
	// nil when none. An augmented item cannot be dropped, traded, or
	// sold, and only a private warehouse (not a public one) accepts it.
	Augmentation *Augmentation

	// persist is fired after every mutation that changes state the items
	// table records, so lazy persistence follows the mutation itself
	// rather than each call site remembering to schedule a write. nil
	// (an instance nothing persists, or any domain test) is a silent
	// no-op.
	persist func(*Instance)
}

// InstanceState is a point-in-time copy of an instance's mutable live state,
// plus the immutable ids callers usually need alongside it.
type InstanceState struct {
	ObjectID   int32
	TemplateID int32
	OwnerID    int32

	Count        int
	EnchantLevel int

	Location     Location
	LocationData int

	CustomType1, CustomType2 int
	ManaLeft                 int
	Time                     int64

	ShotsMask    int32
	Augmentation *Augmentation
}

func (inst *Instance) lock() *sync.RWMutex {
	if mu := atomic.LoadPointer(&inst.mu); mu != nil {
		return (*sync.RWMutex)(mu)
	}
	mu := &sync.RWMutex{}
	if atomic.CompareAndSwapPointer(&inst.mu, nil, unsafe.Pointer(mu)) {
		return mu
	}
	return (*sync.RWMutex)(atomic.LoadPointer(&inst.mu))
}

// Snapshot returns a race-free copy of inst's current persisted and transient
// state. Augmentation is copied so callers can safely pass the snapshot across
// package boundaries.
func (inst *Instance) Snapshot() InstanceState {
	if inst == nil {
		return InstanceState{}
	}
	mu := inst.lock()
	mu.RLock()
	defer mu.RUnlock()

	st := InstanceState{
		ObjectID:     inst.ObjectID,
		TemplateID:   inst.TemplateID,
		OwnerID:      inst.OwnerID,
		Count:        inst.Count,
		EnchantLevel: inst.EnchantLevel,
		Location:     inst.Location,
		LocationData: inst.LocationData,
		CustomType1:  inst.CustomType1,
		CustomType2:  inst.CustomType2,
		ManaLeft:     inst.ManaLeft,
		Time:         inst.Time,
		ShotsMask:    inst.ShotsMask,
		Augmentation: inst.Augmentation,
	}
	if inst.Augmentation != nil {
		aug := *inst.Augmentation
		st.Augmentation = &aug
	}
	return st
}

// Clone returns a detached copy of inst's current state. The clone has its own
// lock and can be passed to persistence without racing with live mutations.
func (inst *Instance) Clone() *Instance {
	if inst == nil {
		return nil
	}
	st := inst.Snapshot()
	return st.Instance()
}

// Instance rebuilds a detached item instance from st.
func (st InstanceState) Instance() *Instance {
	inst := &Instance{
		ObjectID:     st.ObjectID,
		TemplateID:   st.TemplateID,
		OwnerID:      st.OwnerID,
		Count:        st.Count,
		EnchantLevel: st.EnchantLevel,
		Location:     st.Location,
		LocationData: st.LocationData,
		CustomType1:  st.CustomType1,
		CustomType2:  st.CustomType2,
		ManaLeft:     st.ManaLeft,
		Time:         st.Time,
		ShotsMask:    st.ShotsMask,
		Augmentation: st.Augmentation,
	}
	if st.Augmentation != nil {
		aug := *st.Augmentation
		inst.Augmentation = &aug
	}
	return inst
}

// Equipped reports whether st describes an item occupying a paperdoll or
// pet equipment slot.
func (st InstanceState) Equipped() bool {
	return st.Location == LocationPaperdoll || st.Location == LocationPetEquip
}

// SetPersistNotifier records the hook fired whenever a mutation changes
// state the items table records. Passing nil clears it.
func (inst *Instance) SetPersistNotifier(notify func(*Instance)) {
	if inst == nil {
		return
	}
	mu := inst.lock()
	mu.Lock()
	defer mu.Unlock()
	inst.persist = notify
}

// persisted fires inst's persistence notifier. Call it after releasing
// inst's lock, so a hook that reads inst back can't deadlock against the
// mutation that fired it.
//
// That is the only lock the hook is guaranteed to be clear of: callers in
// itemcontainer fire these mutations with the container's own lock held
// (Container.Add, Container.DestroyAllItems, Inventory.Restore), so a
// persister must not reach back into the container that installed it.
func (inst *Instance) persisted() {
	mu := inst.lock()
	mu.RLock()
	notify := inst.persist
	mu.RUnlock()
	if notify != nil {
		notify(inst)
	}
}

// CountValue returns inst's current count.
func (inst *Instance) CountValue() int {
	mu := inst.lock()
	mu.RLock()
	defer mu.RUnlock()
	return inst.Count
}

// AddCount adds delta to inst's count and returns the resulting value.
func (inst *Instance) AddCount(delta int) int {
	mu := inst.lock()
	mu.Lock()
	inst.Count += delta
	count := inst.Count
	mu.Unlock()

	if delta != 0 {
		inst.persisted()
	}
	return count
}

// ReduceCount subtracts count when enough units are present and returns the
// remaining count. It leaves inst unchanged and returns ok=false otherwise.
func (inst *Instance) ReduceCount(count int) (remaining int, ok bool) {
	if count <= 0 {
		return inst.CountValue(), false
	}
	mu := inst.lock()
	mu.Lock()
	if inst.Count < count {
		remaining = inst.Count
		mu.Unlock()
		return remaining, false
	}
	inst.Count -= count
	remaining = inst.Count
	mu.Unlock()

	inst.persisted()
	return remaining, true
}

// DestroyState marks inst as no longer owned or persisted as a live item row.
func (inst *Instance) DestroyState() {
	mu := inst.lock()
	mu.Lock()
	inst.Count = 0
	inst.OwnerID = 0
	inst.Location = LocationVoid
	inst.LocationData = 0
	mu.Unlock()

	inst.persisted()
}

// SetOwner records inst's owning object without touching location, for a
// ground item's temporary loot-protection owner.
func (inst *Instance) SetOwner(ownerID int32) {
	mu := inst.lock()
	mu.Lock()
	changed := inst.OwnerID != ownerID
	inst.OwnerID = ownerID
	mu.Unlock()

	if changed {
		inst.persisted()
	}
}

// SetOwnerLocation records inst's owning object and location data.
func (inst *Instance) SetOwnerLocation(ownerID int32, loc Location, locData int) {
	mu := inst.lock()
	mu.Lock()
	changed := inst.OwnerID != ownerID || inst.Location != loc || inst.LocationData != locData
	inst.OwnerID = ownerID
	inst.Location = loc
	inst.LocationData = locData
	mu.Unlock()

	if changed {
		inst.persisted()
	}
}

// SetLocation records inst's current location while preserving its owner.
func (inst *Instance) SetLocation(loc Location, locData int) {
	mu := inst.lock()
	mu.Lock()
	changed := inst.Location != loc || inst.LocationData != locData
	inst.Location = loc
	inst.LocationData = locData
	mu.Unlock()

	if changed {
		inst.persisted()
	}
}

// SetEnchantLevel changes inst's enchant level. It reports whether anything
// changed.
func (inst *Instance) SetEnchantLevel(level int) bool {
	mu := inst.lock()
	mu.Lock()
	if inst.EnchantLevel == level {
		mu.Unlock()
		return false
	}
	inst.EnchantLevel = level
	mu.Unlock()

	inst.persisted()
	return true
}

// SetCustomType2 changes the second custom-type flag and schedules persistence.
func (inst *Instance) SetCustomType2(value int) bool {
	mu := inst.lock()
	mu.Lock()
	if inst.CustomType2 == value {
		mu.Unlock()
		return false
	}
	inst.CustomType2 = value
	mu.Unlock()

	inst.persisted()
	return true
}

// DecreaseMana reduces a shadow item's remaining mana by amount, floored at
// zero, and returns the resulting mana value.
func (inst *Instance) DecreaseMana(amount int) int {
	mu := inst.lock()
	mu.Lock()
	if inst.ManaLeft < 0 || amount <= 0 {
		manaLeft := inst.ManaLeft
		mu.Unlock()
		return manaLeft
	}
	if amount > inst.ManaLeft {
		amount = inst.ManaLeft
	}
	inst.ManaLeft -= amount
	manaLeft := inst.ManaLeft
	mu.Unlock()

	if amount > 0 {
		inst.persisted()
	}
	return manaLeft
}

// Equipped reports whether inst currently occupies a paperdoll (or pet
// equip) slot.
func (inst *Instance) Equipped() bool {
	mu := inst.lock()
	mu.RLock()
	defer mu.RUnlock()
	return InstanceState{Location: inst.Location}.Equipped()
}

// Augmented reports whether inst carries an augmentation.
func (inst *Instance) Augmented() bool {
	mu := inst.lock()
	mu.RLock()
	defer mu.RUnlock()
	return inst.Augmentation != nil
}

// Dropable reports whether inst can be dropped on the ground: augmented
// items never can, regardless of what tmpl allows.
func (inst *Instance) Dropable(tmpl *Template) bool {
	return !inst.Augmented() && tmpl.Dropable
}

// Tradable reports whether inst can be offered in a trade: augmented items
// never can, regardless of what tmpl allows.
func (inst *Instance) Tradable(tmpl *Template) bool {
	return !inst.Augmented() && tmpl.Tradable
}

// Sellable reports whether inst can be sold to a store: augmented items
// never can, regardless of what tmpl allows.
func (inst *Instance) Sellable(tmpl *Template) bool {
	return !inst.Augmented() && tmpl.Sellable
}

// Destroyable reports whether inst can be destroyed: quest items never can,
// regardless of what tmpl allows.
func (inst *Instance) Destroyable(tmpl *Template) bool {
	return !inst.QuestItem(tmpl) && tmpl.Destroyable
}

// QuestItem reports whether inst is a quest item per its etc-item detail.
func (inst *Instance) QuestItem(tmpl *Template) bool {
	return tmpl.EtcItem != nil && tmpl.EtcItem.IsQuestItem()
}

// Depositable reports whether inst can be stored in a warehouse or freight.
// An equipped item never can. A private warehouse additionally accepts any
// otherwise-depositable item; a public one (clan warehouse, freight) also
// requires the item to be tradable and not a shadow item.
func (inst *Instance) Depositable(tmpl *Template, privateWarehouse bool) bool {
	if inst.Equipped() || !tmpl.Depositable {
		return false
	}
	return privateWarehouse || (inst.Tradable(tmpl) && !inst.ShadowItem(tmpl))
}

// ShadowItem reports whether inst is a time-limited shadow item: tmpl
// declares a non-negative duration.
func (inst *Instance) ShadowItem(tmpl *Template) bool {
	return tmpl.Duration > -1
}

// InitialManaLeft returns the ManaLeft a freshly created instance of t
// should start at: t's full duration in seconds for a shadow item, or -1
// (no mana tracking at all) for a regular one.
func (t *Template) InitialManaLeft() int {
	if t.Duration > -1 {
		return int(t.Duration) * 60
	}
	return -1
}

// DisplayedManaLeft returns inst's remaining shadow-item mana rounded down
// to whole minutes for client display, or -1 when tmpl isn't a shadow item.
func (inst *Instance) DisplayedManaLeft(tmpl *Template) int {
	if !inst.ShadowItem(tmpl) {
		return -1
	}
	mu := inst.lock()
	mu.RLock()
	defer mu.RUnlock()
	return inst.ManaLeft / 60
}

// NewStackOrEquip builds the Instance for one starter-item grant: objectID
// is the pre-allocated id for the new item row, tmpl is its template, count
// how many to grant, and equip whether the caller wants it worn rather than
// left in the general inventory. equip only takes effect when tmpl can
// actually occupy an equipment slot; slot is the paperdoll position to
// record when it does (see Slot.PaperdollIndex).
//
// Paired slots (either ear, either ring) are not resolved here — granting
// two starter items for the same paired slot is not a case the shipped
// profession data produces, and general equip-conflict resolution belongs
// to the inventory system, not character creation.
func NewStackOrEquip(objectID int32, tmpl *Template, count int, equip bool) Instance {
	inst := Instance{
		ObjectID:   objectID,
		TemplateID: tmpl.ID,
		Count:      count,
		ManaLeft:   -1,
		Location:   LocationInventory,
	}

	if equip && tmpl.Equipable() {
		if slot, ok := tmpl.Slot.PaperdollIndex(); ok {
			inst.Location = LocationPaperdoll
			inst.LocationData = slot
		}
	}

	return inst
}
