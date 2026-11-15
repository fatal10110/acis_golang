package dynamic

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
)

// multilayerWith returns a multilayer block whose cell (0, 0) holds layers,
// bottom-up, and every other cell one open layer at height 0.
func multilayerWith(t *testing.T, layers ...int16) block.Block {
	t.Helper()
	var cells [block.CellCount][]block.Cell
	for i := range cells {
		cells[i] = []block.Cell{{Height: 0, NSWE: block.AllDirections}}
	}
	cells[0] = nil
	for _, h := range layers {
		cells[0] = append(cells[0], block.Cell{Height: h, NSWE: block.AllDirections})
	}
	base, err := block.NewMultilayer(cells)
	if err != nil {
		t.Fatalf("NewMultilayer: %v", err)
	}
	return base
}

// touching overlays cell (0, 0) with an object that leaves every height
// alone: it closes no direction and stands far from the cell's layers.
func touching(b *Block) {
	b.Add(&stubObject{z: 10_000, height: 10, data: [][]block.NSWE{{block.North | block.South | block.West}}})
}

// TestBlockBelowOnOverlayReturnsTopmostLayerBelow pins the layer under a
// height on a cell an object overlays: the scan runs from the top down, so
// it is the highest layer below, as on the static block.
func TestBlockBelowOnOverlayReturnsTopmostLayerBelow(t *testing.T) {
	b := NewBlock(0, 0, multilayerWith(t, 0, 100))
	touching(b)

	layer := b.Below(0, 0, 200)
	if layer < 0 {
		t.Fatal("Below(z=200) found no layer")
	}
	if got := b.Height(layer); got != 100 {
		t.Fatalf("Height(Below(z=200)) = %d, want 100 (the highest layer below)", got)
	}
}

// TestBlockNearestOnOverlayScansInFileOrder pins the nearest layer of a
// cell an object overlays to the scan that walks the layers top-down and
// stops at the first one farther than the last: an exact midpoint
// resolves to the lower layer, and a cell whose layers are out of order
// stops at the first local minimum.
func TestBlockNearestOnOverlayScansInFileOrder(t *testing.T) {
	for _, tc := range []struct {
		name   string
		layers []int16 // bottom-up storage order
		z      int32
		want   int16
	}{
		{"midpoint of two", []int16{0, 100}, 50, 0},
		{"below midpoint", []int16{0, 100}, 49, 0},
		{"above midpoint", []int16{0, 100}, 51, 100},
		{"midpoint of the upper pair", []int16{0, 100, 200}, 150, 100},
		// Top-down order 50, 0, 100: the scan stops at 50 when 0 is farther.
		{"out of order", []int16{100, 0, 50}, 60, 50},
		{"out of order near top", []int16{100, 0, 50}, 90, 50},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := NewBlock(0, 0, multilayerWith(t, tc.layers...))
			touching(b)
			if got := b.HeightNearest(0, 0, tc.z); got != tc.want {
				t.Errorf("HeightNearest(z=%d) = %d, want %d", tc.z, got, tc.want)
			}
			if got := b.Height(b.Nearest(0, 0, tc.z)); got != tc.want {
				t.Errorf("Height(Nearest(z=%d)) = %d, want %d", tc.z, got, tc.want)
			}
		})
	}
}

// TestBlockLiftOnSingleLayerCellOfMultilayerBlock pins the height a closed
// object gives a cell it fills: on a multilayer block the lift stops a
// cell's ignore height short of the layer above the object's foot, even
// when that is the cell's own single layer; on a complex block it reaches
// the object's top.
func TestBlockLiftOnSingleLayerCellOfMultilayerBlock(t *testing.T) {
	inside := func() *stubObject {
		return &stubObject{z: 0, height: 200, data: [][]block.NSWE{{block.NoDirections}}}
	}

	multi := NewBlock(0, 0, multilayerWith(t, 10))
	multi.Add(inside())
	if got := multi.HeightNearest(0, 0, 0); got != 10-block.CellIgnoreHeight {
		t.Errorf("multilayer single-layer cell height = %d, want %d", got, 10-block.CellIgnoreHeight)
	}
	if got := multi.NSWENearest(0, 0, 0); got != block.NoDirections {
		t.Errorf("multilayer single-layer cell nswe = %v, want none", got)
	}

	var cells [block.CellCount]block.Cell
	for i := range cells {
		cells[i] = block.Cell{Height: 10, NSWE: block.AllDirections}
	}
	cx := NewBlock(0, 0, block.NewComplex(cells))
	cx.Add(inside())
	if got := cx.HeightNearest(0, 0, 0); got != 200 {
		t.Errorf("complex cell height = %d, want the object top 200", got)
	}
}
