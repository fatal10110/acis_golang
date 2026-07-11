package itemcontainer

import "github.com/fatal10110/acis_golang/internal/gameserver/model/item"

// Freight is a player's freight container: items ordered from a merchant
// for pickup at a specific castle town. Unlike a plain Container, its
// contents are filtered by which town is currently active — an item
// deposited before per-town freight existed (LocationData 0) stays
// visible everywhere, but everything else only shows up while its own
// town is active.
//
// Freight can't simply embed Container and override its accessors (Go
// method sets don't support that kind of override), so it exposes its own
// Visible* methods instead; the embedded Container's own Items/Size/etc.
// still see every item regardless of town, which restore/persistence
// code needs.
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
	n := 0
	for _, inst := range f.Items() {
		if f.visible(inst) {
			n++
		}
	}
	return n
}

// VisibleItems returns every item visible at the currently active town.
func (f *Freight) VisibleItems() []*item.Instance {
	var out []*item.Instance
	for _, inst := range f.Items() {
		if f.visible(inst) {
			out = append(out, inst)
		}
	}
	return out
}

// VisibleItemByTemplateID returns the first instance of templateID visible
// at the currently active town, or nil.
func (f *Freight) VisibleItemByTemplateID(templateID int32) *item.Instance {
	for _, inst := range f.ItemsByTemplateID(templateID) {
		if f.visible(inst) {
			return inst
		}
	}
	return nil
}

// AddNew creates a new instance of templateID and adds it, tagging it with
// the currently active town so it's scoped like every other town-tagged
// freight item.
func (f *Freight) AddNew(templateID int32, count int, objectID int32) *item.Instance {
	inst := f.Container.AddNew(templateID, count, objectID)
	if inst != nil && f.ActiveLocation > 0 {
		inst.LocationData = f.ActiveLocation
	}
	return inst
}

// ValidateCapacity reports whether adding slotCount more stacks/instances
// keeps the town-visible portion of the freight within SlotLimit.
func (f *Freight) ValidateCapacity(slotCount int) bool {
	if slotCount == 0 || f.SlotLimit <= 0 {
		return true
	}
	return f.VisibleSize()+slotCount <= f.SlotLimit
}
