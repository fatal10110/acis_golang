package itemcontainer

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

func newFullFreight(size int) *Freight {
	f := NewFreight(0x10000001, testTemplates())
	f.ActiveLocation = 1
	for i := 0; i < size; i++ {
		inst := f.AddNew(daggerTemplateID, 1, int32(0x20000000+i))
		if i%2 == 0 {
			inst.LocationData = 2
		}
	}
	return f
}

func newWeightedInventory(size int) *Inventory {
	templates := item.NewTable([]*item.Template{
		{ID: daggerTemplateID, Kind: item.KindWeapon, Slot: item.SlotRHand, Weight: 3, Weapon: &item.WeaponDetail{}},
	})
	inv := NewPlayerInventory(0x10000001, templates)
	for i := 0; i < size; i++ {
		inv.AddNew(daggerTemplateID, 1, int32(0x20000000+i))
	}
	return inv
}

func TestFreight_VisibleItemsAllocatesOnlyResultSlice(t *testing.T) {
	f := newFullFreight(64)

	allocs := testing.AllocsPerRun(100, func() {
		_ = f.VisibleItems()
	})
	if allocs > 1 {
		t.Fatalf("VisibleItems() allocs/run = %.0f, want at most 1 result-slice allocation", allocs)
	}
}

func TestInventory_UpdateWeightDoesNotAllocateForIteration(t *testing.T) {
	inv := newWeightedInventory(64)

	allocs := testing.AllocsPerRun(100, func() {
		_ = inv.UpdateWeight()
	})
	if allocs != 0 {
		t.Fatalf("UpdateWeight() allocs/run = %.0f, want 0", allocs)
	}
}

func BenchmarkFreightValidateCapacity(b *testing.B) {
	f := newFullFreight(128)
	f.SlotLimit = 256

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = f.ValidateCapacity(1)
	}
}

func BenchmarkInventoryUpdateWeight(b *testing.B) {
	inv := newWeightedInventory(128)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = inv.UpdateWeight()
	}
}
