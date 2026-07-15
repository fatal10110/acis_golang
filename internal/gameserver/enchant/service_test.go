package enchant

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

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
	want := []StepKind{StepSystemMessage, StepInventoryUpdate, StepEnchantResult, StepBroadcastEquipment}
	if !sameStepKinds(res.Steps, want) || res.Steps[2].EnchantResult != ResultSuccess {
		t.Fatalf("steps = %+v, want success flow", res.Steps)
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
