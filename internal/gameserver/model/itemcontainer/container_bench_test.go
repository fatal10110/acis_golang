package itemcontainer

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// ---- from container_bench_test.go ----
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

// TestFreight_VisibleItemsAllocationsDoNotScaleWithSize guards what the
// allocation budget is actually for: VisibleItems must not allocate per item.
// It allocates twice — the result slice, and the ordering-key slice
// sortContainerOrder copies entry times into so the comparison never reads
// live state — and both are one allocation each however many items are held.
// Comparing two sizes pins that, where a bare count would quietly admit a
// per-item allocation the next time the budget is raised.
func TestFreight_VisibleItemsAllocationsDoNotScaleWithSize(t *testing.T) {
	measure := func(size int) float64 {
		f := newFullFreight(size)
		return testing.AllocsPerRun(100, func() {
			_ = f.VisibleItems()
		})
	}

	small, large := measure(8), measure(64)
	if small != large {
		t.Fatalf("VisibleItems() allocs/run = %.0f at 8 items, %.0f at 64; allocations must not scale with size", small, large)
	}
	if large > 2 {
		t.Fatalf("VisibleItems() allocs/run = %.0f, want at most the result slice plus the ordering-key slice", large)
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
