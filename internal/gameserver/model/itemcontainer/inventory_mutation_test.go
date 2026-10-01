package itemcontainer

import (
	"slices"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

func TestInventory_DropItem_PartialSplitsIntoNewInstance(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
	})
	inv := NewPlayerInventory(0x10000001, templates)
	inst := inv.AddNew(1, 100, 0x20000001)

	dropped := inv.DropItem(inst.ObjectID, 30, 0x30000001)
	if dropped == nil || dropped.ObjectID != 0x30000001 || dropped.Count != 30 {
		t.Fatalf("DropItem() = %+v, want a new instance carrying 30 units", dropped)
	}
	if inst.Count != 70 {
		t.Errorf("remaining stack Count = %d, want 70", inst.Count)
	}
	if inv.ItemByObjectID(inst.ObjectID) != inst {
		t.Errorf("the original stack should stay in the inventory")
	}
}

func TestInventory_DropItem_FullyRemovesInstance(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{}},
	})
	inv := NewPlayerInventory(0x10000001, templates)
	inst := inv.AddNew(1, 1, 0x20000001)
	tmpl, _ := templates.Get(1)
	inv.EquipItem(inst, tmpl)

	dropped := inv.DropItem(inst.ObjectID, 1, 0)
	if dropped != inst {
		t.Fatalf("DropItem() = %+v, want the original instance back", dropped)
	}
	if inst.OwnerID != 0 || inst.Location != item.LocationVoid {
		t.Errorf("dropped instance state = %+v, want OwnerID=0 Location=VOID", inst)
	}
	if inv.ItemAt(RHand) != nil {
		t.Errorf("dropping an equipped item should unequip it first")
	}
	if inv.Size() != 0 {
		t.Errorf("Size() = %d, want 0", inv.Size())
	}
}

func TestInventory_DestroyByTemplateID_QueuesUpdateAndUnequips(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
		{ID: 2, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{}},
	})
	inv := NewPlayerInventory(0x10000001, templates)
	stack := inv.AddNew(1, 10, 0x20000001)
	inv.DrainUpdates()

	if got := inv.DestroyByTemplateID(1, 3); got != stack || stack.Count != 7 {
		t.Fatalf("DestroyByTemplateID() = %+v, Count = %d, want the stack with 7 left", got, stack.Count)
	}
	updates := inv.DrainUpdates()
	if len(updates) != 1 || updates[0].State != UpdateModified || updates[0].ObjectID != stack.ObjectID || updates[0].Count != 7 {
		t.Fatalf("updates after partial destroy = %+v, want one modified update with Count=7", updates)
	}

	weapon := inv.AddNew(2, 1, 0x20000002)
	tmpl, _ := templates.Get(2)
	inv.EquipItem(weapon, tmpl)
	inv.DrainUpdates()

	if got := inv.DestroyByObjectID(weapon.ObjectID, 1); got != weapon {
		t.Fatalf("DestroyByObjectID() = %+v, want the weapon instance", got)
	}
	if inv.ItemAt(RHand) != nil {
		t.Errorf("fully destroying an equipped item should unequip it first")
	}
	updates = inv.DrainUpdates()
	if len(updates) != 2 || updates[0].State != UpdateModified || updates[1].State != UpdateRemoved || updates[1].ObjectID != weapon.ObjectID {
		t.Fatalf("updates after full destroy = %+v, want an unequip-modified update then a removed update for the weapon", updates)
	}
}

func TestInventory_DestroyAllAndDestroyAllItems_UnequipAndQueueUpdates(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
		{ID: 2, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{}},
	})
	inv := NewPlayerInventory(0x10000001, templates)
	stack := inv.AddNew(1, 10, 0x20000001)
	weapon := inv.AddNew(2, 1, 0x20000002)
	tmpl, _ := templates.Get(2)
	inv.EquipItem(weapon, tmpl)
	inv.DrainUpdates()

	if got := inv.DestroyAll(weapon); got != weapon {
		t.Fatalf("DestroyAll() = %+v, want the weapon instance", got)
	}
	if inv.ItemAt(RHand) != nil {
		t.Errorf("DestroyAll on an equipped item should unequip it first")
	}
	updates := inv.DrainUpdates()
	if len(updates) != 2 || updates[0].State != UpdateModified || updates[1].State != UpdateRemoved || updates[1].ObjectID != weapon.ObjectID {
		t.Fatalf("updates after DestroyAll = %+v, want an unequip-modified update then a removed update", updates)
	}

	inv.DestroyAllItems()
	if inv.Size() != 0 {
		t.Errorf("Size() = %d after DestroyAllItems, want 0", inv.Size())
	}
	updates = inv.DrainUpdates()
	if len(updates) != 1 || updates[0].State != UpdateRemoved || updates[0].ObjectID != stack.ObjectID {
		t.Fatalf("updates after DestroyAllItems = %+v, want one removed update for the remaining stack", updates)
	}
}

// TestInventory_DestroyAllItems_RaceWithAdd runs Add and DestroyAllItems
// concurrently under -race so a snapshot-and-delete phase that isn't
// actually atomic with Add's own map write (a plain data race on
// Container.items, not just a logical ordering question) gets caught. It
// also checks the functional postcondition once the concurrent Adds have
// stopped: a final DestroyAllItems clears everything.
func TestInventory_DestroyAllItems_RaceWithAdd(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindEtcItem, EtcItem: &item.EtcItemDetail{}},
	})
	inv := NewPlayerInventory(0x10000001, templates)

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for id := int32(0x20000001); ; id++ {
			select {
			case <-stop:
				return
			default:
				inv.AddNew(1, 1, id)
			}
		}
	}()

	for range 100 {
		inv.DestroyAllItems()
	}
	close(stop)
	<-done

	inv.DestroyAllItems()
	if inv.Size() != 0 {
		t.Fatalf("Size() = %d after a final DestroyAllItems, want 0", inv.Size())
	}
}

// TestInventory_DestroyAllItems_RaceWithAddAndEquip races an Add-then-equip
// loop against DestroyAllItems and then checks that the survivors' equip
// state is consistent: an item still held and recorded at a paperdoll
// position must be that position's occupant. Clearing the
// item map and the paperdoll in separate critical sections let an item
// added and equipped between them survive in the map while its slot was
// wiped, leaving it equipped by location but absent from the paperdoll.
// The interleaving is timing-dependent, so this is a stress check rather
// than a deterministic reproduction.
func TestInventory_DestroyAllItems_RaceWithAddAndEquip(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 2, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{}},
	})
	tmpl, _ := templates.Get(2)
	inv := NewPlayerInventory(0x10000001, templates)

	var orphans atomic.Int64
	done := make(chan struct{})
	go func() {
		defer close(done)
		for id := int32(0x20000001); id < 0x20000001+20000; id++ {
			inst := inv.AddNew(2, 1, id)
			inv.EquipItem(inst, tmpl)
			// Only this goroutine equips, so once inst has left the right
			// hand only DestroyAllItems can have moved it, and that call
			// must also have taken it out of the item map. Reading the slot
			// first keeps a destroy that lands between these reads from
			// looking like a violation.
			if inv.ItemAt(RHand) != inst && inv.ItemByObjectID(id) == inst && inst.Snapshot().Location == item.LocationPaperdoll {
				orphans.Add(1)
			}
		}
	}()

	for destroying := true; destroying; {
		select {
		case <-done:
			destroying = false
		default:
			inv.DestroyAllItems()
		}
	}

	if n := orphans.Load(); n > 0 {
		t.Errorf("%d items were left held and located in the paperdoll after DestroyAllItems cleared their slot", n)
	}
}

func TestInventory_TransferItemPartialQueuesSourceAndTargetUpdates(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
	})
	playerInv := NewPlayerInventory(0x10000001, templates)
	petInv := NewPetInventory(0x20000001, templates)
	source := playerInv.AddNew(1, 100, 0x30000001)
	playerInv.DrainUpdates()

	result, freedID, freed := playerInv.TransferItem(source.ObjectID, 30, petInv, 0x40000001)
	if result == nil || result.ObjectID != 0x40000001 || result.Count != 30 {
		t.Fatalf("TransferItem() result = %+v, want new pet stack with 30 units", result)
	}
	if freed || freedID != 0 {
		t.Fatalf("TransferItem() freed = (%d, %v), want none for partial transfer", freedID, freed)
	}
	if source.Count != 70 {
		t.Fatalf("source Count = %d, want 70", source.Count)
	}

	sourceUpdates := playerInv.DrainUpdates()
	if len(sourceUpdates) != 1 || sourceUpdates[0].State != UpdateModified || sourceUpdates[0].ObjectID != source.ObjectID || sourceUpdates[0].Count != 70 {
		t.Fatalf("source updates = %+v, want one modified update for remaining source stack", sourceUpdates)
	}
	targetUpdates := petInv.DrainUpdates()
	if len(targetUpdates) != 1 || targetUpdates[0].State != UpdateAdded || targetUpdates[0].ObjectID != result.ObjectID || targetUpdates[0].Count != 30 {
		t.Fatalf("target updates = %+v, want one added update for pet stack", targetUpdates)
	}
}

func TestInventory_TransferItemFullIntoExistingStackQueuesRemoveAndModify(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
	})
	playerInv := NewPlayerInventory(0x10000001, templates)
	petInv := NewPetInventory(0x20000001, templates)
	source := playerInv.AddNew(1, 20, 0x30000001)
	existing := petInv.AddNew(1, 5, 0x40000001)
	playerInv.DrainUpdates()
	petInv.DrainUpdates()

	result, freedID, freed := playerInv.TransferItem(source.ObjectID, 20, petInv, 0)
	if result != existing {
		t.Fatalf("TransferItem() result = %+v, want existing pet stack", result)
	}
	if !freed || freedID != source.ObjectID {
		t.Fatalf("TransferItem() freed = (%d, %v), want source object freed", freedID, freed)
	}
	if existing.Count != 25 {
		t.Fatalf("existing pet stack Count = %d, want 25", existing.Count)
	}
	if playerInv.ItemByObjectID(source.ObjectID) != nil {
		t.Fatalf("source stack should leave player inventory")
	}

	sourceUpdates := playerInv.DrainUpdates()
	if len(sourceUpdates) != 1 || sourceUpdates[0].State != UpdateRemoved || sourceUpdates[0].ObjectID != source.ObjectID || sourceUpdates[0].Count != 20 {
		t.Fatalf("source updates = %+v, want one removed update for original source stack", sourceUpdates)
	}
	targetUpdates := petInv.DrainUpdates()
	if len(targetUpdates) != 1 || targetUpdates[0].State != UpdateModified || targetUpdates[0].ObjectID != existing.ObjectID || targetUpdates[0].Count != 25 {
		t.Fatalf("target updates = %+v, want one modified update for merged pet stack", targetUpdates)
	}
}

type removalRecorder struct {
	inventoryDeliveryRecorder
	removed []int32
}

func (r *removalRecorder) InventoryItemsRemoved(_ *Inventory, objectIDs []int32) {
	r.removed = append(r.removed, objectIDs...)
}

// TestInventory_RemovalDeliveryHearsEveryInstanceThatLeaves pins which
// mutations report an instance leaving: a whole destroy, drop, transfer,
// exchange move and DestroyAllItems do; a partial count change of a stack
// that stays does not.
func TestInventory_RemovalDeliveryHearsEveryInstanceThatLeaves(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
		{ID: 2, Kind: item.KindEtcItem, EtcItem: &item.EtcItemDetail{}},
	})
	const owner = 0x10000001
	rec := &removalRecorder{}
	inv := NewPlayerInventoryWithDelivery(owner, templates, rec, nil)
	other := NewPlayerInventory(owner+1, templates)

	stack := inv.AddNew(1, 10, 0x30000001)
	single := inv.AddNew(2, 1, 0x30000002)
	dropped := inv.AddNew(2, 1, 0x30000003)
	moved := inv.AddNew(2, 1, 0x30000004)
	traded := inv.AddNew(2, 1, 0x30000005)
	rest := inv.AddNew(2, 1, 0x30000006)
	expect := func(step string, want ...int32) {
		t.Helper()
		if !slices.Equal(rec.removed, want) {
			t.Fatalf("%s: removed = %v, want %v", step, rec.removed, want)
		}
		rec.removed = nil
	}

	inv.DestroyItem(stack, 4)
	inv.DropItem(stack.ObjectID, 2, 0x30000010)
	inv.TransferItem(stack.ObjectID, 1, other, 0x30000011)
	expect("partial changes")

	inv.DestroyItem(single, 1)
	expect("whole destroy", single.ObjectID)
	inv.DropItem(dropped.ObjectID, 1, 0)
	expect("drop", dropped.ObjectID)
	inv.TransferItem(moved.ObjectID, 1, other, 0)
	expect("transfer", moved.ObjectID)
	Exchange(inv, other, func(a, b Held) {
		if _, ok := a.Transfer(traded.ObjectID, 1, b, 0); !ok {
			t.Fatal("exchange transfer failed")
		}
	})
	expect("exchange", traded.ObjectID)
	inv.DestroyAllItems()
	if len(rec.removed) != 2 || !slices.Contains(rec.removed, stack.ObjectID) || !slices.Contains(rec.removed, rest.ObjectID) {
		t.Fatalf("DestroyAllItems: removed = %v, want %d and %d", rec.removed, stack.ObjectID, rest.ObjectID)
	}
}
