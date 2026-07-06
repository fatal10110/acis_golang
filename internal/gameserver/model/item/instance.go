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
