package item

// Instance is one persisted item: a template plus the owner-specific state
// recorded in the items table (how many, where it sits, its enchant level).
type Instance struct {
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
}

// DecreaseMana reduces a shadow item's remaining mana by amount, floored at
// zero.
func (inst *Instance) DecreaseMana(amount int) {
	if inst.ManaLeft < 0 || amount <= 0 {
		return
	}
	if amount > inst.ManaLeft {
		amount = inst.ManaLeft
	}
	inst.ManaLeft -= amount
}

// Equipped reports whether inst currently occupies a paperdoll (or pet
// equip) slot.
func (inst *Instance) Equipped() bool {
	return inst.Location == LocationPaperdoll || inst.Location == LocationPetEquip
}

// Augmented reports whether inst carries an augmentation.
func (inst *Instance) Augmented() bool {
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
