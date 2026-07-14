package grounditem

import (
	"fmt"
	"sync/atomic"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Item is an item instance currently placed in the visible world.
//
// Position and visibility are guarded by the embedded world.Presence.
type Item struct {
	world.Presence

	Instance item.Instance
	Template *item.Template

	destroyProtected bool
	dropperID        atomic.Int32
}

// New creates a visible-world item from a persisted instance and its loaded
// template.
func New(inst item.Instance, tmpl *item.Template) (*Item, error) {
	if tmpl == nil {
		return nil, fmt.Errorf("ground item %d: nil template", inst.ObjectID)
	}
	if inst.TemplateID != tmpl.ID {
		return nil, fmt.Errorf("ground item %d: template %d does not match instance template %d", inst.ObjectID, tmpl.ID, inst.TemplateID)
	}
	return &Item{Instance: inst, Template: tmpl}, nil
}

// ObjectID returns the world object id assigned to this item.
func (i *Item) ObjectID() int32 { return i.Instance.ObjectID }

// ItemID returns the static item template id.
func (i *Item) ItemID() int32 { return i.Instance.TemplateID }

// Count returns the item stack count.
func (i *Item) Count() int { return i.Instance.Count }

// Stackable reports whether the item should display its count on the ground.
func (i *Item) Stackable() bool {
	return i != nil && i.Template != nil && i.Template.Stackable
}

// Equipable reports whether the item can occupy an equipment slot.
func (i *Item) Equipable() bool {
	return i != nil && i.Template != nil && i.Template.Equipable()
}

// Herb reports whether this item is an herb.
func (i *Item) Herb() bool {
	return i != nil &&
		i.Template != nil &&
		i.Template.EtcItem != nil &&
		i.Template.EtcItem.Type == item.EtcItemHerb
}

// SetDestroyProtected marks whether the item is exempt from ground cleanup.
func (i *Item) SetDestroyProtected(protected bool) {
	i.destroyProtected = protected
}

// DestroyProtected reports whether the item is exempt from ground cleanup.
func (i *Item) DestroyProtected() bool {
	return i != nil && i.destroyProtected
}

// SetDropperID records the actor whose drop animation should be shown while
// this item is being spawned.
func (i *Item) SetDropperID(id int32) {
	i.dropperID.Store(id)
}

// DropperID returns the temporary dropper object id used by DropItem.
func (i *Item) DropperID() int32 {
	if i == nil {
		return 0
	}
	return i.dropperID.Load()
}

// Snapshot captures this ground item's persisted state and remaining destroy
// interval in milliseconds.
func (i *Item) Snapshot(timeLeftMillis int64) item.GroundSnapshot {
	x, y, z := i.Position()
	return item.GroundSnapshot{
		Instance:       i.Instance,
		X:              x,
		Y:              y,
		Z:              z,
		TimeLeftMillis: timeLeftMillis,
	}
}
