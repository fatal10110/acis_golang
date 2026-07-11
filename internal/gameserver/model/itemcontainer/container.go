package itemcontainer

import (
	"cmp"
	"slices"
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// Container is one owned collection of item instances sitting at a single
// item.Location: a private warehouse, a clan warehouse, or freight. An
// equip-capable collection (a player or pet's own inventory) is an
// Inventory, which wraps a Container and adds paperdoll slots.
//
// SlotLimit caps how many item stacks/instances the container can hold; 0
// means unlimited, matching the base behavior every container has until a
// caller sets a real limit sourced from wherever that limit eventually
// comes from (player status, clan config, ...) — this package doesn't load
// config itself.
//
// mu guards items.
type Container struct {
	ownerID   int32
	location  item.Location
	templates *item.Table

	SlotLimit int

	mu    sync.RWMutex
	items map[int32]*item.Instance
}

// NewContainer returns an empty container owned by ownerID, holding items
// at location, resolving templates against templates.
func NewContainer(ownerID int32, location item.Location, templates *item.Table) *Container {
	return &Container{
		ownerID:   ownerID,
		location:  location,
		templates: templates,
		items:     make(map[int32]*item.Instance),
	}
}

// NewWarehouse returns an empty private warehouse container for ownerID.
func NewWarehouse(ownerID int32, templates *item.Table) *Container {
	return NewContainer(ownerID, item.LocationWarehouse, templates)
}

// NewClanWarehouse returns an empty clan warehouse container for clanID.
func NewClanWarehouse(clanID int32, templates *item.Table) *Container {
	return NewContainer(clanID, item.LocationClanWarehouse, templates)
}

// OwnerID returns the owning actor's object id.
func (c *Container) OwnerID() int32 { return c.ownerID }

// Location returns the item.Location this container's own items sit at.
func (c *Container) Location() item.Location { return c.location }

// Templates returns the template table this container resolves item ids
// against.
func (c *Container) Templates() *item.Table { return c.templates }

// Size returns the number of item instances the container holds.
func (c *Container) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Items returns every item instance the container holds, ordered by object
// id for determinism (the Java reference orders by most-recently-touched;
// nothing in this package's scope depends on that order, so object id is
// used instead as a simpler, stable substitute).
func (c *Container) Items() []*item.Instance {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.itemsLocked()
}

func (c *Container) itemsLocked() []*item.Instance {
	out := make([]*item.Instance, 0, len(c.items))
	for _, inst := range c.items {
		out = append(out, inst)
	}
	slices.SortFunc(out, func(a, b *item.Instance) int { return cmp.Compare(a.ObjectID, b.ObjectID) })
	return out
}

// HasItem reports whether the container holds any instance of templateID.
func (c *Container) HasItem(templateID int32) bool {
	return c.ItemByTemplateID(templateID) != nil
}

// HasItems reports whether the container holds at least one instance of
// every id in templateIDs.
func (c *Container) HasItems(templateIDs ...int32) bool {
	for _, id := range templateIDs {
		if !c.HasItem(id) {
			return false
		}
	}
	return true
}

// HasAnyItem reports whether the container holds at least one instance of
// any id in templateIDs.
func (c *Container) HasAnyItem(templateIDs ...int32) bool {
	for _, id := range templateIDs {
		if c.HasItem(id) {
			return true
		}
	}
	return false
}

// ItemsByTemplateID returns every instance of templateID the container
// holds, ordered by object id.
func (c *Container) ItemsByTemplateID(templateID int32) []*item.Instance {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var out []*item.Instance
	for _, inst := range c.items {
		if inst.TemplateID == templateID {
			out = append(out, inst)
		}
	}
	slices.SortFunc(out, func(a, b *item.Instance) int { return cmp.Compare(a.ObjectID, b.ObjectID) })
	return out
}

// ItemByTemplateID returns the first instance of templateID the container
// holds, or nil if it holds none.
func (c *Container) ItemByTemplateID(templateID int32) *item.Instance {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.itemByTemplateIDLocked(templateID)
}

func (c *Container) itemByTemplateIDLocked(templateID int32) *item.Instance {
	for _, inst := range c.items {
		if inst.TemplateID == templateID {
			return inst
		}
	}
	return nil
}

// ItemByObjectID returns the instance identified by objectID, or nil if the
// container doesn't hold it.
func (c *Container) ItemByObjectID(objectID int32) *item.Instance {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.items[objectID]
}

// ItemCount reports how many units of templateID the container holds.
// enchantLevel restricts the match to that exact enchant level, or matches
// any level when negative. includeEquipped controls whether an equipped
// instance counts. A stackable match returns that single stack's count
// directly (the container invariant is that at most one stack of a given
// template/enchant combination ever coexists); a non-stackable match
// accumulates one per matching instance.
func (c *Container) ItemCount(templateID int32, enchantLevel int, includeEquipped bool) int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	count := 0
	for _, inst := range c.items {
		if inst.TemplateID != templateID {
			continue
		}
		if enchantLevel >= 0 && inst.EnchantLevel != enchantLevel {
			continue
		}
		if !includeEquipped && inst.Equipped() {
			continue
		}
		tmpl, _ := c.templates.Get(inst.TemplateID)
		if tmpl != nil && tmpl.Stackable {
			return inst.Count
		}
		count++
	}
	return count
}

// Adena returns the container's adena count.
func (c *Container) Adena() int {
	return c.ItemCount(item.AdenaID, -1, true)
}

// Add adds inst to the container, merging into an existing stack of the
// same template when one already exists and the template is stackable.
// When merged, inst's own identity is absorbed into the pre-existing
// stack: absorbed is true and the caller must release inst's object id
// back to the id allocator (and remove it from the world registry) since
// it's no longer live. The returned instance is always the one the
// container now actually holds.
func (c *Container) Add(inst *item.Instance) (result *item.Instance, absorbed bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	tmpl, _ := c.templates.Get(inst.TemplateID)
	if old := c.itemByTemplateIDLocked(inst.TemplateID); old != nil && tmpl != nil && tmpl.Stackable {
		old.Count += inst.Count
		return old, true
	}

	inst.OwnerID = c.ownerID
	inst.Location = c.location
	inst.LocationData = 0
	c.items[inst.ObjectID] = inst
	return inst, false
}

// AddNew creates a new instance of templateID, using objectID as its
// pre-allocated world id, and adds it to the container the same way Add
// does. count is clamped to at least 1. It returns nil when templateID
// isn't a loaded template.
//
// The Java reference can split a non-stackable count > 1 across several
// freshly created instances when MULTIPLE_ITEM_DROP is enabled; this
// always creates exactly one instance instead (a stackable template gets
// count units on it, a non-stackable one gets a single unit regardless of
// count) — a deliberate simplification, since that config path only
// matters for bulk GM item creation.
func (c *Container) AddNew(templateID int32, count int, objectID int32) *item.Instance {
	tmpl, ok := c.templates.Get(templateID)
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
	result, _ := c.Add(inst)
	return result
}

// Remove removes inst from the container, leaving its ownership and
// location fields untouched — a plain container never resets them itself
// (Inventory does, but only after it has also cleared any paperdoll slot
// inst occupied; see Inventory.Remove). It reports whether inst was
// actually held.
func (c *Container) Remove(inst *item.Instance) bool {
	c.mu.Lock()
	_, ok := c.items[inst.ObjectID]
	if ok {
		delete(c.items, inst.ObjectID)
	}
	c.mu.Unlock()
	return ok
}

// DestroyItem destroys count units of inst: when inst holds more than
// count, its count is simply reduced and inst is returned; when it holds
// exactly count, inst is removed from the container, its state reset (as
// if destroyed), and returned; when it holds fewer than count, nothing
// changes and nil is returned.
//
// The caller remains responsible for releasing inst's object id and
// removing it from the world registry once this returns inst with a
// now-zero count.
func (c *Container) DestroyItem(inst *item.Instance, count int) *item.Instance {
	if inst == nil {
		return nil
	}
	if inst.Count > count {
		inst.Count -= count
		return inst
	}
	if inst.Count < count {
		return nil
	}
	if !c.Remove(inst) {
		return nil
	}
	inst.Count = 0
	inst.OwnerID = 0
	inst.Location = item.LocationVoid
	return inst
}

// DestroyByObjectID destroys count units of the instance identified by
// objectID, per DestroyItem.
func (c *Container) DestroyByObjectID(objectID int32, count int) *item.Instance {
	return c.DestroyItem(c.ItemByObjectID(objectID), count)
}

// DestroyByTemplateID destroys count units of the first instance of
// templateID found, per DestroyItem.
func (c *Container) DestroyByTemplateID(templateID int32, count int) *item.Instance {
	return c.DestroyItem(c.ItemByTemplateID(templateID), count)
}

// DestroyAll destroys every unit of inst.
func (c *Container) DestroyAll(inst *item.Instance) *item.Instance {
	if inst == nil {
		return nil
	}
	return c.DestroyItem(inst, inst.Count)
}

// DestroyAllItems destroys every item instance the container holds.
func (c *Container) DestroyAllItems() {
	for _, inst := range c.Items() {
		c.DestroyAll(inst)
	}
}

// Transfer moves count units of the instance identified by objectID from
// c into target, merging into an existing stack in target when the item
// is stackable and target already holds one. newObjectID supplies the
// pre-allocated world id for a brand new instance in target, used only
// when one must be created (a non-stackable item, or a partial-count
// transfer of a stackable item into a target that holds none yet);
// otherwise it's unused. It returns the resulting instance in target
// (nil if objectID isn't held by c), and reports via freed/freedObjectID
// an object id the caller must now release — either objectID itself
// (fully absorbed into an existing target stack or fully destroyed here)
// or none.
//
// The caller remains responsible for undoing any life-stone augmentation
// bonus a transferred instance was granting its previous owner — that's
// stat-engine behavior this package doesn't own.
func (c *Container) Transfer(objectID int32, count int, target *Container, newObjectID int32) (result *item.Instance, freedObjectID int32, freed bool) {
	src := c.ItemByObjectID(objectID)
	if src == nil {
		return nil, 0, false
	}

	tmpl, _ := c.templates.Get(src.TemplateID)
	stackable := tmpl != nil && tmpl.Stackable

	var targetItem *item.Instance
	if stackable {
		targetItem = target.ItemByTemplateID(src.TemplateID)
	}

	if count > src.Count {
		count = src.Count
	}

	if src.Count == count && targetItem == nil {
		c.Remove(src)
		result, _ = target.Add(src)
		return result, 0, false
	}

	if src.Count > count {
		src.Count -= count
	} else {
		c.Remove(src)
		src.Count = 0
		src.OwnerID = 0
		src.Location = item.LocationVoid
		freedObjectID, freed = objectID, true
	}

	if targetItem != nil {
		targetItem.Count += count
		return targetItem, freedObjectID, freed
	}

	return target.AddNew(src.TemplateID, count, newObjectID), freedObjectID, freed
}

// ValidateCapacity reports whether adding slotCount more stacks/instances
// keeps the container within SlotLimit. A SlotLimit of 0 means unlimited.
func (c *Container) ValidateCapacity(slotCount int) bool {
	if slotCount == 0 || c.SlotLimit <= 0 {
		return true
	}
	return c.Size()+slotCount <= c.SlotLimit
}
