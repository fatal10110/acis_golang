package dynamic

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
)

func TestBlockAppendActiveCellsCopiesOnlyActiveOverrides(t *testing.T) {
	base := block.NewFlat(0)
	b := NewBlock(0, 0, base)
	if b.Base() != block.Block(base) {
		t.Fatal("Base() does not return the wrapped static block")
	}
	obj := &stubObject{height: 32, data: [][]block.NSWE{{block.NoDirections}}}

	if got, ok := b.AppendActiveCells(nil, 0, 0); ok || len(got) != 0 {
		t.Fatalf("untouched cell: AppendActiveCells = %v, %v; want empty, false", got, ok)
	}

	b.Add(obj)
	var buf [block.MaxLayers]block.Cell
	got, ok := b.AppendActiveCells(buf[:0], 0, 0)
	if !ok || !slices.Equal(got, b.Cells(0, 0)) {
		t.Fatalf("active cell: AppendActiveCells = %v, %v; want %v, true", got, ok, b.Cells(0, 0))
	}
	if got[0].NSWE != block.NoDirections {
		t.Fatalf("active cell NSWE = %v, want none", got[0].NSWE)
	}
	if got, ok := b.AppendActiveCells(nil, 1, 0); ok || len(got) != 0 {
		t.Fatalf("untouched cell next to object: AppendActiveCells = %v, %v; want empty, false", got, ok)
	}
	allocs := testing.AllocsPerRun(1000, func() {
		_, _ = b.AppendActiveCells(buf[:0], 0, 0)
	})
	if allocs != 0 {
		t.Fatalf("AppendActiveCells allocations = %.0f, want 0", allocs)
	}

	b.Remove(obj)
	if got, ok := b.AppendActiveCells(nil, 0, 0); ok || len(got) != 0 {
		t.Fatalf("after Remove: AppendActiveCells = %v, %v; want empty, false", got, ok)
	}
}
