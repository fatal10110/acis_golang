package itemcontainer

import (
	"slices"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// nowMillis stamps an item as it enters a container. Containers order their
// contents newest-first, so this is the ordering key, not just bookkeeping.
func nowMillis() int64 { return time.Now().UnixMilli() }

// orderedItem is an instance paired with its ordering key, copied out once
// so the comparison never re-reads live state.
type orderedItem struct {
	inst *item.Instance
	time int64
}

func keyed(inst *item.Instance) orderedItem {
	return orderedItem{inst: inst, time: inst.TimeValue()}
}

// byContainerOrder is the total order every container lists its contents in:
// descending entry time, then descending object id. That puts the most
// recently acquired item first, which is the order the client expects an
// item list in.
func byContainerOrder(a, b orderedItem) int {
	if d := cmpDesc(a.time, b.time); d != 0 {
		return d
	}
	return cmpDesc(int64(a.inst.ObjectID), int64(b.inst.ObjectID))
}

// sortContainerOrder puts items in byContainerOrder, reading each entry time
// exactly once and sorting the copies. Comparing against live state instead
// would make the sort depend on no writer touching Time until it finished:
// slices.SortFunc does not fault on a key that moves mid-sort, it just
// returns a wrong order, which leaves as a silently wrong ItemList. Copying
// the key also keeps Items() to one instance RLock per item rather than one
// per comparison, on a path FrameInventoryUpdate walks per item mutation.
func sortContainerOrder(items []*item.Instance) {
	order := make([]orderedItem, len(items))
	for i, inst := range items {
		order[i] = keyed(inst)
	}
	slices.SortFunc(order, byContainerOrder)
	for i, o := range order {
		items[i] = o.inst
	}
}

func cmpDesc(a, b int64) int {
	switch {
	case a > b:
		return -1
	case a < b:
		return 1
	default:
		return 0
	}
}

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
// mu guards the membership map. Mutable item fields are guarded by
// item.Instance once an instance is visible outside construction/restore code.
type Container struct {
	ownerID   int32
	location  item.Location
	templates *item.Table

	SlotLimit int

	mu      sync.RWMutex
	items   map[int32]*item.Instance
	persist item.Persister
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

// NewContainerWithPersister returns an empty container whose items persist
// through persist.
func NewContainerWithPersister(ownerID int32, location item.Location, templates *item.Table, persist item.Persister) *Container {
	c := NewContainer(ownerID, location, templates)
	c.persist = persist
	return c
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

// ReleasePersistence ends the container's persistence dependency when its
// owner is torn down: the items it still holds stop scheduling writes and a
// later Add binds nothing. Items that already left keep the persister they
// carry, so a transfer out never loses coverage; work already queued stays
// flushable.
func (c *Container) ReleasePersistence() {
	c.mu.Lock()
	defer c.mu.Unlock()
	p := c.persist
	c.persist = nil
	for _, inst := range c.items {
		inst.ReleasePersister(p)
	}
}

// Size returns the number of item instances the container holds.
func (c *Container) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Items returns every item instance the container holds in
// byContainerOrder: newest entry first, object id descending within a tie.
// Packets built straight from this slice (ItemList, TradeStart,
// PackageSendableList) inherit that order, which is the whole point of it.
func (c *Container) Items() []*item.Instance {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.itemsLocked()
}

// ItemsUnordered returns the container's contents in no particular order,
// for callers that index them rather than list them. InventoryUpdate builds
// a lookup map from the result and never reads the sequence, so paying for
// byContainerOrder there would sort a slice purely to iterate it once — on a
// path that runs per item mutation.
func (c *Container) ItemsUnordered() []*item.Instance {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]*item.Instance, 0, len(c.items))
	for _, inst := range c.items {
		out = append(out, inst)
	}
	return out
}

func (c *Container) itemsLocked() []*item.Instance {
	out := make([]*item.Instance, 0, len(c.items))
	for _, inst := range c.items {
		out = append(out, inst)
	}
	sortContainerOrder(out)
	return out
}

func (c *Container) forEach(fn func(*item.Instance)) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, inst := range c.items {
		fn(inst)
	}
}

// HasItem reports whether the container holds any instance of templateID.
// It answers from the first match rather than through ItemByTemplateID:
// existence doesn't depend on which instance is found, and HasItems /
// HasAnyItem / quest conditions call it per template id.
func (c *Container) HasItem(templateID int32) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, inst := range c.items {
		if inst.TemplateID == templateID {
			return true
		}
	}
	return false
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
// holds, in byContainerOrder.
func (c *Container) ItemsByTemplateID(templateID int32) []*item.Instance {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var out []*item.Instance
	for _, inst := range c.items {
		if inst.TemplateID == templateID {
			out = append(out, inst)
		}
	}
	sortContainerOrder(out)
	return out
}

// ItemByTemplateID returns the first instance of templateID the container
// holds, or nil if it holds none.
func (c *Container) ItemByTemplateID(templateID int32) *item.Instance {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.itemByTemplateIDLocked(templateID)
}

// itemByTemplateIDLocked returns the first matching instance in
// byContainerOrder. "First" has to mean the same thing it does in Items(),
// or destroying "the" instance of a template would pick an arbitrary one of
// several — a different enchant level than the player watched disappear.
// It scans for the ordering-best candidate rather than sorting, so callers
// that only want one instance don't pay for a slice and a sort.
func (c *Container) itemByTemplateIDLocked(templateID int32) *item.Instance {
	var best orderedItem
	for _, inst := range c.items {
		if inst.TemplateID != templateID {
			continue
		}
		if cand := keyed(inst); best.inst == nil || byContainerOrder(cand, best) < 0 {
			best = cand
		}
	}
	return best.inst
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
		st := inst.Snapshot()
		if enchantLevel >= 0 && st.EnchantLevel != enchantLevel {
			continue
		}
		if !includeEquipped && st.Equipped() {
			continue
		}
		tmpl, _ := c.templates.Get(inst.TemplateID)
		if tmpl != nil && tmpl.Stackable {
			return st.Count
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
// stack: absorbed is true, inst's own state is reset as destroyed — so a
// row it may have had is deleted rather than surviving alongside the units
// now counted on the pre-existing stack — and the caller must release its
// object id back to the id allocator (and remove it from the world
// registry) since it's no longer live. The returned instance is always the
// one the container now actually holds.
func (c *Container) Add(inst *item.Instance) (result *item.Instance, absorbed bool) {
	if inst == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	tmpl, _ := c.templates.Get(inst.TemplateID)
	if old := c.itemByTemplateIDLocked(inst.TemplateID); old != nil && tmpl != nil && tmpl.Stackable {
		old.AddCount(inst.Snapshot().Count)
		inst.DestroyState()
		return old, true
	}
	if inst.ObjectID == 0 {
		return nil, false
	}

	c.insertLocked(inst)
	return inst, false
}

// insertLocked makes inst one of c's items. The caller holds c.mu.
func (c *Container) insertLocked(inst *item.Instance) {
	// Hand the item this container's persister before the move itself
	// mutates it, so the ownership/location change that brings it in is the
	// first thing reported. A container with no persister of its own leaves
	// the one the item already carries in place: moving between containers
	// never unregisters an item.
	inst.BindPersister(c.persist)
	inst.EnterContainer(c.ownerID, c.location, 0, nowMillis())
	c.items[inst.ObjectID] = inst
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
	inst, ok := newInstance(c.templates, templateID, count, objectID)
	if !ok {
		return nil
	}
	result, _ := c.Add(inst)
	return result
}

func newInstance(templates *item.Table, templateID int32, count int, objectID int32) (*item.Instance, bool) {
	tmpl, ok := templates.Get(templateID)
	if !ok {
		return nil, false
	}
	if count < 1 {
		count = 1
	}
	if !tmpl.Stackable {
		count = 1
	}
	return &item.Instance{
		ObjectID:   objectID,
		TemplateID: templateID,
		Count:      count,
		ManaLeft:   tmpl.InitialManaLeft(),
	}, true
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
	if inst == nil || count <= 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.items[inst.ObjectID] != inst {
		return nil
	}
	return destroyItemCore(inst, count, func(inst *item.Instance) bool {
		delete(c.items, inst.ObjectID)
		return true
	}, nil)
}

func destroyItemCore(inst *item.Instance, count int, remove func(*item.Instance) bool, modified func(*item.Instance)) *item.Instance {
	held := inst.CountValue()
	if held > count {
		if _, ok := inst.ReduceCount(count); !ok {
			return nil
		}
		if modified != nil {
			modified(inst)
		}
		return inst
	}
	if held < count {
		return nil
	}
	if !remove(inst) {
		return nil
	}
	inst.DestroyState()
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
	return c.DestroyItem(inst, inst.CountValue())
}

// DestroyAllItems destroys every item instance the container holds.
func (c *Container) DestroyAllItems() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for objectID, inst := range c.items {
		delete(c.items, objectID)
		inst.DestroyState()
	}
}

type transferTarget interface {
	Add(inst *item.Instance) (result *item.Instance, absorbed bool)
	AddNew(templateID int32, count int, objectID int32) *item.Instance
	ItemByTemplateID(templateID int32) *item.Instance
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
func (c *Container) Transfer(objectID int32, count int, target transferTarget, newObjectID int32) (result *item.Instance, freedObjectID int32, freed bool) {
	if target == nil || count <= 0 {
		return nil, 0, false
	}

	c.mu.Lock()
	src := c.items[objectID]
	if src == nil {
		c.mu.Unlock()
		return nil, 0, false
	}
	st := src.Snapshot()
	templateID := st.TemplateID
	tmpl, _ := c.templates.Get(templateID)
	stackable := tmpl != nil && tmpl.Stackable
	if count > st.Count {
		count = st.Count
	}
	sourceCount := st.Count
	c.mu.Unlock()

	var targetItem *item.Instance
	if stackable {
		targetItem = target.ItemByTemplateID(templateID)
	}

	c.mu.Lock()
	src = c.items[objectID]
	if src == nil {
		c.mu.Unlock()
		return nil, 0, false
	}
	st = src.Snapshot()
	if st.TemplateID != templateID || st.Count != sourceCount {
		c.mu.Unlock()
		return nil, 0, false
	}
	if count > st.Count {
		count = st.Count
	}
	if st.Count == count && targetItem == nil {
		delete(c.items, objectID)
		c.mu.Unlock()
		result, _ = target.Add(src)
		return result, 0, false
	}

	if st.Count > count {
		if _, ok := src.ReduceCount(count); !ok {
			c.mu.Unlock()
			return nil, 0, false
		}
	} else {
		delete(c.items, objectID)
		src.DestroyState()
		freedObjectID, freed = objectID, true
	}
	c.mu.Unlock()

	if targetItem != nil {
		result, _ = target.Add(&item.Instance{ObjectID: newObjectID, TemplateID: templateID, Count: count, ManaLeft: -1})
		return result, freedObjectID, freed
	}

	return target.AddNew(templateID, count, newObjectID), freedObjectID, freed
}

// ValidateCapacity reports whether adding slotCount more stacks/instances
// keeps the container within SlotLimit. A SlotLimit of 0 means unlimited.
func (c *Container) ValidateCapacity(slotCount int) bool {
	if slotCount == 0 || c.SlotLimit <= 0 {
		return true
	}
	return c.Size()+slotCount <= c.SlotLimit
}
