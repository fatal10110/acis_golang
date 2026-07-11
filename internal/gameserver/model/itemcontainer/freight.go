package itemcontainer

import "github.com/fatal10110/acis_golang/internal/gameserver/model/item"

// Freight is a player's freight container: items ordered from a merchant
// for pickup at a specific castle town. Unlike a plain Container, its
// contents are filtered by which town is currently active — an item
// deposited before per-town freight existed (LocationData 0) stays
// visible everywhere, but everything else only shows up while its own
// town is active.
//
// Freight exposes Visible* methods for town-filtered reads. It also provides
// its own Add/AddNew/ItemByTemplateID so transfers use active-town semantics;
// the embedded Container's Items/Size/ItemsByTemplateID still see every item
// regardless of town, which restore/persistence code needs.
type Freight struct {
	*Container

	// ActiveLocation is the currently selected castle town id. Zero means
	// no town is selected, in which case every item is visible.
	ActiveLocation int
}

// NewFreight returns an empty freight container for ownerID.
func NewFreight(ownerID int32, templates *item.Table) *Freight {
	return &Freight{Container: NewContainer(ownerID, item.LocationFreight, templates)}
}

func (f *Freight) visible(inst *item.Instance) bool {
	return inst.LocationData == 0 || f.ActiveLocation == 0 || inst.LocationData == f.ActiveLocation
}

// VisibleSize returns the number of items visible at the currently active
// town.
func (f *Freight) VisibleSize() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	n := 0
	for _, inst := range f.items {
		if f.visible(inst) {
			n++
		}
	}
	return n
}

// VisibleItems returns every item visible at the currently active town.
func (f *Freight) VisibleItems() []*item.Instance {
	f.mu.RLock()
	defer f.mu.RUnlock()
	var out []*item.Instance
	for _, inst := range f.itemsLocked() {
		if f.visible(inst) {
			out = append(out, inst)
		}
	}
	return out
}

// VisibleItemByTemplateID returns the first instance of templateID visible
// at the currently active town, or nil.
func (f *Freight) VisibleItemByTemplateID(templateID int32) *item.Instance {
	return f.ItemByTemplateID(templateID)
}

// ItemByTemplateID returns the first visible instance of templateID, or nil.
func (f *Freight) ItemByTemplateID(templateID int32) *item.Instance {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.itemByTemplateIDLocked(templateID)
}

func (f *Freight) itemByTemplateIDLocked(templateID int32) *item.Instance {
	for _, inst := range f.itemsLocked() {
		if inst.TemplateID == templateID && f.visible(inst) {
			return inst
		}
	}
	return nil
}

// Add adds inst to the currently active town's visible freight contents.
func (f *Freight) Add(inst *item.Instance) (result *item.Instance, absorbed bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	tmpl, _ := f.templates.Get(inst.TemplateID)
	if old := f.itemByTemplateIDLocked(inst.TemplateID); old != nil && tmpl != nil && tmpl.Stackable {
		old.Count += inst.Count
		return old, true
	}

	inst.OwnerID = f.ownerID
	inst.Location = f.location
	if f.ActiveLocation > 0 {
		inst.LocationData = f.ActiveLocation
	} else {
		inst.LocationData = 0
	}
	f.items[inst.ObjectID] = inst
	return inst, false
}

// AddNew creates a new instance of templateID and adds it, tagging it with
// the currently active town so it's scoped like every other town-tagged
// freight item.
func (f *Freight) AddNew(templateID int32, count int, objectID int32) *item.Instance {
	tmpl, ok := f.Templates().Get(templateID)
	if !ok {
		return nil
	}
	if count < 1 {
		count = 1
	}
	if !tmpl.Stackable {
		count = 1
	}

	inst := &item.Instance{
		ObjectID:   objectID,
		TemplateID: templateID,
		Count:      count,
		ManaLeft:   tmpl.InitialManaLeft(),
	}
	result, _ := f.Add(inst)
	return result
}

// ValidateCapacity reports whether adding slotCount more stacks/instances
// keeps the town-visible portion of the freight within SlotLimit.
func (f *Freight) ValidateCapacity(slotCount int) bool {
	if slotCount == 0 || f.SlotLimit <= 0 {
		return true
	}
	return f.VisibleSize()+slotCount <= f.SlotLimit
}
