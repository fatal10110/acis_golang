package itemcontainer

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// recordingPersister is a Persister that records what it is asked
// to schedule.
type recordingPersister struct {
	ids       []int32
	states    []item.InstanceState
	onPersist func(*item.Instance)
}

func (p *recordingPersister) Persist(inst *item.Instance) {
	p.ids = append(p.ids, inst.ObjectID)
	p.states = append(p.states, inst.Snapshot())
	if p.onPersist != nil {
		p.onPersist(inst)
	}
}

// TestContainerItemPersisterCoversAddedItems proves an item entering a
// wired container reports both the move that brought it in and every later
// mutation, so persistence follows the item rather than the call site.
func TestContainerItemPersisterCoversAddedItems(t *testing.T) {
	rec := &recordingPersister{}
	c := NewContainerWithPersister(0x10000001, item.LocationWarehouse, testTemplates(), rec)

	inst := c.AddNew(potionTemplateID, 5, 0x20000001)
	if inst == nil {
		t.Fatal("AddNew() = nil")
	}
	if len(rec.ids) != 1 || rec.ids[0] != 0x20000001 {
		t.Fatalf("after AddNew, changed = %v, want [0x20000001]", rec.ids)
	}

	// A count change made with no client involved must still be reported.
	inst.ReduceCount(2)
	if len(rec.ids) != 2 {
		t.Fatalf("after ReduceCount, changed = %v, want two entries", rec.ids)
	}

	// So must the destruction that removes it, since the row has to be
	// deleted rather than left behind.
	if got := c.DestroyAll(inst); got == nil {
		t.Fatal("DestroyAll() = nil")
	}
	if len(rec.ids) != 3 {
		t.Fatalf("after DestroyAll, changed = %v, want three entries", rec.ids)
	}
}

// TestContainerAddKeepsPersisterOfUnwiredDestination pins that moving an
// item into a container nothing has wired — a warehouse, a freight, a pet
// inventory before its owner logs in — neither swallows the move nor
// silences the item afterwards. Dropping the hook there would leave the
// items row pointing at the container the item just left.
func TestContainerAddKeepsPersisterOfUnwiredDestination(t *testing.T) {
	rec := &recordingPersister{}
	source := NewContainerWithPersister(0x10000001, item.LocationWarehouse, testTemplates(), rec)

	inst := source.AddNew(daggerTemplateID, 1, 0x20000001)
	if inst == nil {
		t.Fatal("AddNew() = nil")
	}
	if !source.Remove(inst) {
		t.Fatal("Remove() = false")
	}
	before := len(rec.ids)

	// A destination with no persister of its own.
	target := NewContainer(0x10000002, item.LocationWarehouse, testTemplates())
	if _, absorbed := target.Add(inst); absorbed {
		t.Fatal("Add() absorbed a non-stackable item")
	}
	if len(rec.ids) != before+1 {
		t.Fatalf("moving into an unwired container reported %d changes, want 1", len(rec.ids)-before)
	}

	inst.SetEnchantLevel(3)
	if len(rec.ids) != before+2 {
		t.Errorf("mutating after the move reported %d changes, want 1", len(rec.ids)-before-1)
	}
}

// TestContainerAddAbsorbedItemReportsDestruction covers the merge path: the
// absorbed instance's units are now counted on the pre-existing stack, so
// any row it had must be deleted rather than left behind double-counting
// them after a restart.
func TestContainerAddAbsorbedItemReportsDestruction(t *testing.T) {
	c := NewContainerWithPersister(0x10000001, item.LocationWarehouse, testTemplates(), &recordingPersister{})

	if first := c.AddNew(adenaTemplateID, 100, 0x20000001); first == nil {
		t.Fatal("AddNew() = nil")
	}

	// An incoming stack that already has a row of its own.
	incoming := &item.Instance{ObjectID: 0x20000002, TemplateID: adenaTemplateID, Count: 50, OwnerID: 0x10000009, Location: item.LocationInventory, ManaLeft: -1}
	incomingRec := &recordingPersister{}
	incoming.BindPersister(incomingRec)
	reported := &incomingRec.states

	result, absorbed := c.Add(incoming)
	if !absorbed {
		t.Fatal("Add() did not absorb a stackable item")
	}
	if got := result.CountValue(); got != 150 {
		t.Errorf("merged stack count = %d, want 150", got)
	}
	if len(*reported) == 0 {
		t.Fatal("absorbed item reported no change; its row would survive the merge")
	}
	last := (*reported)[len(*reported)-1]
	if last.Count != 0 || last.Location != item.LocationVoid {
		t.Errorf("absorbed item reported count=%d loc=%v, want a destroyed state", last.Count, last.Location)
	}
}

// TestFreightAddAbsorbedItemReportsDestruction covers the same merge path
// through Freight's own Add.
func TestFreightAddAbsorbedItemReportsDestruction(t *testing.T) {
	f := NewFreightWithPersister(0x10000001, testTemplates(), &recordingPersister{})

	if first := f.AddNew(adenaTemplateID, 100, 0x20000001); first == nil {
		t.Fatal("AddNew() = nil")
	}

	incoming := &item.Instance{ObjectID: 0x20000002, TemplateID: adenaTemplateID, Count: 50, OwnerID: 0x10000009, Location: item.LocationInventory, ManaLeft: -1}
	destroyed := false
	incoming.BindPersister(&recordingPersister{onPersist: func(inst *item.Instance) {
		if st := inst.Snapshot(); st.Count == 0 && st.Location == item.LocationVoid {
			destroyed = true
		}
	}})

	if _, absorbed := f.Add(incoming); !absorbed {
		t.Fatal("Add() did not absorb a stackable item")
	}
	if !destroyed {
		t.Error("absorbed freight item never reported its destruction")
	}
}

// TestInventoryItemPersisterAppliesToRestoredItems covers the login order:
// the inventory is built with its persistence dependency and restored from
// its persisted rows. Restoring must not schedule a write of what was just
// read, but the items must be covered from then on.
func TestInventoryItemPersisterAppliesToRestoredItems(t *testing.T) {
	restored := []*item.Instance{
		{ObjectID: 0x20000001, TemplateID: potionTemplateID, Count: 5, Location: item.LocationInventory, ManaLeft: -1},
	}
	rec := &recordingPersister{}
	inv := RestorePlayerInventoryWithDelivery(0x10000001, testTemplates(), restored, nil, rec)
	if len(rec.ids) != 0 {
		t.Fatalf("restoring an inventory persisted %d times, want 0", len(rec.ids))
	}

	held := inv.ItemByObjectID(0x20000001)
	if held == nil {
		t.Fatal("restored item missing from inventory")
	}
	held.AddCount(1)
	if len(rec.ids) != 1 {
		t.Errorf("persist calls after mutating a restored item = %d, want 1", len(rec.ids))
	}
}

func TestInventoryRestoreNormalizesStacksAndEquipment(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: potionTemplateID, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
		{ID: twoHandID, Kind: item.KindWeapon, Slot: item.SlotLRHand, Weapon: &item.WeaponDetail{Type: item.WeaponBigSword}},
		{ID: shieldID, Kind: item.KindArmor, Slot: item.SlotLHand, Armor: &item.ArmorDetail{Type: item.ArmorShield}},
	})
	shield := &item.Instance{ObjectID: 0x20000001, TemplateID: shieldID, Count: 1, Location: item.LocationPaperdoll, LocationData: LHand, ManaLeft: -1}
	twoHand := &item.Instance{ObjectID: 0x20000002, TemplateID: twoHandID, Count: 1, Location: item.LocationPaperdoll, LocationData: RHand, ManaLeft: -1}
	inv := RestorePlayerInventory(0x10000001, templates, []*item.Instance{
		{ObjectID: 0x20000003, TemplateID: potionTemplateID, Count: 4, Location: item.LocationInventory, ManaLeft: -1},
		{ObjectID: 0x20000004, TemplateID: potionTemplateID, Count: 6, Location: item.LocationInventory, ManaLeft: -1},
		shield,
		twoHand,
	})

	if got := inv.ItemCount(potionTemplateID, -1, true); got != 10 {
		t.Errorf("restored potion count = %d, want 10", got)
	}
	if got := inv.Size(); got != 3 {
		t.Errorf("restored size = %d, want 3", got)
	}
	if got := inv.ItemAt(RHand); got != twoHand {
		t.Errorf("RHand = %v, want two-handed weapon", got)
	}
	if got := inv.ItemAt(LHand); got != nil {
		t.Errorf("LHand = %v, want nil", got)
	}
	if got := shield.Snapshot().Location; got != item.LocationInventory {
		t.Errorf("displaced shield location = %v, want inventory", got)
	}
}

// TestInventoryReleasePersistenceStopsHeldItems proves the dependency ends
// with its owner for the items still held, so a logged-out player's items
// stop registering with the task.
func TestInventoryReleasePersistenceStopsHeldItems(t *testing.T) {
	rec := &recordingPersister{}
	inv := NewPlayerInventoryWithDelivery(0x10000001, testTemplates(), nil, rec)

	inst := inv.AddNew(potionTemplateID, 5, 0x20000001)
	if inst == nil {
		t.Fatal("AddNew() = nil")
	}
	before := len(rec.ids)

	inv.ReleasePersistence()
	inst.AddCount(1)
	if len(rec.ids) != before {
		t.Errorf("persist calls after release = %d, want %d", len(rec.ids), before)
	}
}

// TestContainerReleasePersistenceKeepsItemsThatLeft pins that tearing a
// container down only unwires what it still holds: an item that already
// moved into an unwired container keeps its coverage.
func TestContainerReleasePersistenceKeepsItemsThatLeft(t *testing.T) {
	rec := &recordingPersister{}
	source := NewContainerWithPersister(0x10000001, item.LocationInventory, testTemplates(), rec)
	inst := source.AddNew(daggerTemplateID, 1, 0x20000001)
	if inst == nil || !source.Remove(inst) {
		t.Fatal("setup: could not add and remove the item")
	}
	target := NewContainer(0x10000002, item.LocationWarehouse, testTemplates())
	target.Add(inst)
	before := len(rec.ids)

	source.ReleasePersistence()
	inst.SetEnchantLevel(7)
	if got := len(rec.ids) - before; got != 1 {
		t.Errorf("persist calls for an item that left before release = %d, want 1", got)
	}
}

// TestInventoryRestoreMergePersistsBothRows proves a restore that merges
// stacks schedules exactly the merge — the grown stack, then the absorbed
// row's delete — and nothing for a row restored unchanged. Without the
// delete the absorbed row is merged in again on every later restore.
func TestInventoryRestoreMergePersistsBothRows(t *testing.T) {
	rows := []*item.Instance{
		{ObjectID: 0x20000001, TemplateID: adenaTemplateID, Count: 100, Location: item.LocationInventory, ManaLeft: -1},
		{ObjectID: 0x20000002, TemplateID: adenaTemplateID, Count: 50, Location: item.LocationInventory, ManaLeft: -1},
		{ObjectID: 0x20000003, TemplateID: daggerTemplateID, Count: 1, Location: item.LocationInventory, ManaLeft: -1},
	}
	rec := &recordingPersister{}
	inv := RestorePlayerInventoryWithDelivery(0x10000001, testTemplates(), rows, nil, rec)
	if len(rec.states) != 2 {
		t.Fatalf("restore persisted %v, want exactly the survivor and the absorbed row", rec.ids)
	}
	if st := rec.states[0]; st.ObjectID != 0x20000001 || st.Count != 150 {
		t.Errorf("first write = id %#x count %d, want the survivor at 150", st.ObjectID, st.Count)
	}
	if st := rec.states[1]; st.ObjectID != 0x20000002 || st.Count != 0 || st.Location != item.LocationVoid {
		t.Errorf("second write = id %#x count %d loc %v, want the absorbed row destroyed", st.ObjectID, st.Count, st.Location)
	}
	held := inv.ItemByObjectID(0x20000001)
	if held == nil {
		t.Fatal("merged stack missing")
	}
	held.AddCount(1)
	if len(rec.ids) != 3 {
		t.Errorf("persist calls after mutating the merged stack = %d, want 3", len(rec.ids))
	}
}

// TestContainerItemsOrderedByEntryTimeThenObjectID pins byContainerOrder on
// both of its keys at once, with the two disagreeing: entry time descending
// decides first, and only a tie falls through to object id descending. Object
// ids and times are interleaved so an implementation that used either key
// alone would produce a different order than the one asserted.
func TestContainerItemsOrderedByEntryTimeThenObjectID(t *testing.T) {
	c := NewWarehouse(1, testTemplates())
	for _, objectID := range []int32{601, 602, 603, 604} {
		if c.AddNew(daggerTemplateID, 1, objectID) == nil {
			t.Fatalf("AddNew(%d) returned nil", objectID)
		}
	}
	times := map[int32]int64{601: 300, 602: 100, 603: 300, 604: 200}
	for _, inst := range c.Items() {
		inst.SetTime(times[inst.ObjectID])
	}

	got := make([]int32, 0, 4)
	for _, inst := range c.Items() {
		got = append(got, inst.ObjectID)
	}
	want := []int32{603, 601, 604, 602}
	if !slices.Equal(got, want) {
		t.Fatalf("Items() object ids = %v, want %v", got, want)
	}

	// "First instance of this template" has to mean the same thing the list
	// does, or destroying one of several picks an arbitrary instance.
	if got := c.ItemByTemplateID(daggerTemplateID); got == nil || got.ObjectID != want[0] {
		t.Fatalf("ItemByTemplateID() = %v, want the first listed instance %d", got, want[0])
	}
	byTemplate := make([]int32, 0, 4)
	for _, inst := range c.ItemsByTemplateID(daggerTemplateID) {
		byTemplate = append(byTemplate, inst.ObjectID)
	}
	if !slices.Equal(byTemplate, want) {
		t.Fatalf("ItemsByTemplateID() object ids = %v, want %v", byTemplate, want)
	}
}

// TestContainerAddStampsEntryTimeWithoutRestampingMergedStack pins which adds
// move an item to the front of the list. Taking a new instance in stamps it as
// the newest; merging units into a stack that is already held leaves that
// stack's place in the list alone.
func TestContainerAddStampsEntryTimeWithoutRestampingMergedStack(t *testing.T) {
	c := NewWarehouse(1, testTemplates())

	stack := c.AddNew(adenaTemplateID, 100, 700)
	if stack == nil {
		t.Fatal("AddNew(adena) returned nil")
	}
	if stack.TimeValue() == 0 {
		t.Fatal("Add left the item's entry time unstamped")
	}
	stack.SetTime(1000)

	dagger := c.AddNew(daggerTemplateID, 1, 701)
	if dagger == nil {
		t.Fatal("AddNew(dagger) returned nil")
	}
	if dagger.TimeValue() <= 1000 {
		t.Fatalf("second Add stamped entry time %d, want it newer than the first item's 1000", dagger.TimeValue())
	}

	merged, absorbed := c.Add(&item.Instance{ObjectID: 702, TemplateID: adenaTemplateID, Count: 50})
	if !absorbed || merged != stack {
		t.Fatalf("Add(adena) = %v absorbed=%v, want it merged into the held stack", merged, absorbed)
	}
	if stack.TimeValue() != 1000 {
		t.Fatalf("merging units restamped the held stack's entry time to %d, want it left at 1000", stack.TimeValue())
	}
	if first := c.Items()[0]; first != dagger {
		t.Fatalf("Items()[0] = object %d, want the dagger %d: a merge must not move the stack to the front", first.ObjectID, dagger.ObjectID)
	}
}

// TestRestoreMergesDuplicateStacksIntoTheFirstRowRestored pins which of two
// duplicate stackable rows survives a restore: the one the row loop reaches
// first, whatever its object id.
//
// The merge target is never ambiguous, which is why the unordered scan that
// picks it is not a source of nondeterminism. A stackable template can only
// ever have one stack in the container — the first row is inserted, and every
// later duplicate collapses into it — so by the time a merge is resolved
// there is exactly one candidate to find. The reference behaves the same way
// for the same reason: its restore loop resolves the target through
// getItemByItemId against the partially built set, which likewise holds a
// single stack of that template.
func TestRestoreMergesDuplicateStacksIntoTheFirstRowRestored(t *testing.T) {
	for _, restoreOrder := range [][]int32{{801, 802}, {802, 801}} {
		rows := make([]*item.Instance, 0, 2)
		for _, objectID := range restoreOrder {
			rows = append(rows, &item.Instance{
				ObjectID:   objectID,
				TemplateID: adenaTemplateID,
				Count:      100,
				Location:   item.LocationInventory,
			})
		}

		inv := RestorePlayerInventory(1, testTemplates(), rows)

		items := inv.Items()
		if len(items) != 1 {
			t.Fatalf("restore order %v: Items() = %d entries, want the two rows merged into one", restoreOrder, len(items))
		}
		if want := restoreOrder[0]; items[0].ObjectID != want {
			t.Fatalf("restore order %v: surviving object id = %d, want the first row restored %d", restoreOrder, items[0].ObjectID, want)
		}
		if got := items[0].CountValue(); got != 200 {
			t.Fatalf("restore order %v: merged count = %d, want 200", restoreOrder, got)
		}
		if got := inv.ItemByObjectID(restoreOrder[1]); got != nil {
			t.Fatalf("restore order %v: merged-away instance %d still held", restoreOrder, restoreOrder[1])
		}
	}
}
