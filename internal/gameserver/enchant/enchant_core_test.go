package enchant

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// ---- from service_test.go ----
type testIDs struct{ next int32 }

func (ids *testIDs) NextID() (int32, error) {
	ids.next++
	return ids.next, nil
}

func TestStateSelectAndClear(t *testing.T) {
	state := NewState()
	if first := state.Select(1, 600); !first {
		t.Fatal("first Select returned false")
	}
	if first := state.Select(1, 601); first {
		t.Fatal("second Select returned true")
	}
	if got := state.Active(1); got != 601 {
		t.Fatalf("active scroll = %d, want 601", got)
	}
	if cleared := state.Clear(1); !cleared {
		t.Fatal("Clear returned false")
	}
	if got := state.Active(1); got != 0 {
		t.Fatalf("active after clear = %d, want 0", got)
	}
}

func TestServiceSuccessConsumesScrollAndPersistsLevel(t *testing.T) {
	state := NewState()
	templates := testTemplates()
	inv := itemcontainer.NewPlayerInventory(1, templates)
	weapon := inv.AddNew(30, 1, 500)
	scroll := inv.AddNew(955, 1, 600)
	inv.DrainUpdates()
	state.Select(1, scroll.ObjectID)

	res, err := NewService(state, nil, func() float64 { return 0 }).EnchantItem(1, inv, weapon.ObjectID)
	if err != nil {
		t.Fatalf("EnchantItem error = %v", err)
	}
	if weapon.EnchantLevel != 1 {
		t.Fatalf("weapon enchant = %d, want 1", weapon.EnchantLevel)
	}
	if inv.ItemByObjectID(scroll.ObjectID) != nil {
		t.Fatal("scroll still in inventory")
	}
	if state.Active(1) != 0 {
		t.Fatalf("active scroll = %d, want cleared", state.Active(1))
	}
	if len(res.Persist) != 2 || res.Persist[0].Action != inventory.PersistDelete || res.Persist[1].Action != inventory.PersistUpdate {
		t.Fatalf("persist actions = %+v, want scroll delete and weapon update", res.Persist)
	}
	want := []StepKind{StepSystemMessage, StepEnchantResult, StepBroadcastEquipment}
	if !sameStepKinds(res.Steps, want) || res.Steps[1].EnchantResult != ResultSuccess {
		t.Fatalf("steps = %+v, want success flow", res.Steps)
	}
}

// TestConsumedScrollDeleteCarriesPreDestroyOwner pins which persistence lane
// a consumed scroll's delete runs on. DestroyItem takes the exhausted stack
// through item.Instance.DestroyState, which zeroes OwnerID, so an action built
// from the post-destroy snapshot would carry owner 0 and go to lane 0 while
// the same row's earlier updates sit on the owner's lane — and an upserting
// save that overtook the delete there would put the row back.
func TestConsumedScrollDeleteCarriesPreDestroyOwner(t *testing.T) {
	const ownerID = 101
	state := NewState()
	inv := itemcontainer.NewPlayerInventory(ownerID, testTemplates())
	weapon := inv.AddNew(30, 1, 500)
	scroll := inv.AddNew(955, 2, 600)
	inv.DrainUpdates()

	// First enchant leaves one scroll: an update, on the owner's lane.
	state.Select(ownerID, scroll.ObjectID)
	res, err := NewService(state, nil, func() float64 { return 0 }).EnchantItem(ownerID, inv, weapon.ObjectID)
	if err != nil {
		t.Fatalf("first EnchantItem error = %v", err)
	}
	if got := res.Persist[0]; got.Action != inventory.PersistUpdate || got.Item != scroll {
		t.Fatalf("first scroll action = %+v, want an update of the scroll", got)
	}
	if got := scroll.Snapshot().OwnerID; got != ownerID {
		t.Fatalf("remaining scroll owner = %d, want %d", got, ownerID)
	}

	// Second enchant consumes it: the delete of the same row must name the
	// same owner, or it lands on another lane than the update above.
	state.Select(ownerID, scroll.ObjectID)
	res, err = NewService(state, nil, func() float64 { return 0 }).EnchantItem(ownerID, inv, weapon.ObjectID)
	if err != nil {
		t.Fatalf("second EnchantItem error = %v", err)
	}
	del := res.Persist[0]
	if del.Action != inventory.PersistDelete || del.ObjectID != scroll.ObjectID {
		t.Fatalf("second scroll action = %+v, want a delete of object %d", del, scroll.ObjectID)
	}
	if del.OwnerID != ownerID {
		t.Fatalf("consumed scroll delete owner = %d, want %d (the owner the row held)", del.OwnerID, ownerID)
	}
}

func TestServiceNormalFailureAddsCrystalReward(t *testing.T) {
	state := NewState()
	templates := testTemplates()
	inv := itemcontainer.NewPlayerInventory(1, templates)
	weapon := inv.AddNew(30, 1, 500)
	weapon.EnchantLevel = 3
	scroll := inv.AddNew(955, 1, 600)
	inv.DrainUpdates()
	state.Select(1, scroll.ObjectID)

	res, err := NewService(state, &testIDs{next: 700}, func() float64 { return 0.99 }).EnchantItem(1, inv, weapon.ObjectID)
	if err != nil {
		t.Fatalf("EnchantItem error = %v", err)
	}
	if inv.ItemByObjectID(weapon.ObjectID) != nil {
		t.Fatal("weapon still in inventory")
	}
	crystals := inv.ItemByTemplateID(item.CrystalD.ItemID())
	if crystals == nil || crystals.ObjectID != 701 || crystals.Count != 275 {
		t.Fatalf("crystals = %+v, want allocated 275 D crystals", crystals)
	}
	if len(res.Persist) != 3 || res.Persist[0].ObjectID != scroll.ObjectID || res.Persist[1].ObjectID != weapon.ObjectID || res.Persist[2].Action != inventory.PersistSave {
		t.Fatalf("persist actions = %+v, want scroll delete, weapon delete, crystal save", res.Persist)
	}
	if res.Steps[len(res.Steps)-2].EnchantResult != ResultBrokenWithCrystals {
		t.Fatalf("steps = %+v, want broken-with-crystals result before broadcast", res.Steps)
	}
}

func sameStepKinds(steps []Step, want []StepKind) bool {
	if len(steps) != len(want) {
		return false
	}
	for i := range steps {
		if steps[i].Kind != want[i] {
			return false
		}
	}
	return true
}

func testTemplates() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: 30, Kind: item.KindWeapon, Slot: item.SlotRHand, Duration: -1, Crystal: item.CrystalD, CrystalCount: 10, Weapon: &item.WeaponDetail{Type: item.WeaponSword}},
		{ID: 40, Kind: item.KindArmor, Slot: item.SlotChest, Duration: -1, Crystal: item.CrystalD, Armor: &item.ArmorDetail{Type: item.ArmorMagic}},
		{ID: 955, Kind: item.KindEtcItem, Duration: -1, Stackable: true, EtcItem: &item.EtcItemDetail{Type: item.EtcItemScrollEnchantWeapon, Handler: "EnchantScrolls"}},
		{ID: 6575, Kind: item.KindEtcItem, Duration: -1, Stackable: true, EtcItem: &item.EtcItemDetail{Type: item.EtcItemBlessedScrollEnchantWeapon, Handler: "EnchantScrolls"}},
		{ID: item.CrystalD.ItemID(), Kind: item.KindEtcItem, Duration: -1, Stackable: true, EtcItem: &item.EtcItemDetail{}},
	})
}
