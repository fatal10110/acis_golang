package block

import (
	"fmt"
	"math"
)

var _ Block = (*Multilayer)(nil)

// layerSlot is the per-cell multiplier used to pack a (cell, layer
// number) pair into the single opaque handle Nearest/Above/Below return.
// It must exceed MaxLayers so every layer number gets its own slot.
const layerSlot = MaxLayers + 1

// Multilayer is a block whose cells may each have more than one height
// layer (e.g. a bridge over a cave, or a building interior stacked over
// a basement), stored lowest to highest.
type Multilayer struct {
	cells [CellCount][]Cell
}

// NewMultilayer returns a Multilayer block built from the given per-cell
// layers. cells is indexed by cellX*CellsY + cellY, and each cell's
// layers must already be ordered from lowest height to highest. Every
// cell must have between 1 and MaxLayers layers.
func NewMultilayer(cells [CellCount][]Cell) (*Multilayer, error) {
	for i, layers := range cells {
		if len(layers) == 0 || len(layers) > MaxLayers {
			return nil, fmt.Errorf("geo/block: cell %d: invalid layer count %d", i, len(layers))
		}
	}
	return &Multilayer{cells: cells}, nil
}

// Kind identifies b as KindMultilayer.
func (b *Multilayer) Kind() Kind { return KindMultilayer }

// HasGeodata always reports true.
func (b *Multilayer) HasGeodata() bool { return true }

// Layers returns how many height layers the cell at (cellX, cellY) has.
func (b *Multilayer) Layers(cellX, cellY int) int {
	return len(b.cells[cellIndex(cellX, cellY)])
}

// HeightNearest returns the height of the layer, at the given cell, closest to worldZ.
func (b *Multilayer) HeightNearest(cellX, cellY int, worldZ int32) int16 {
	return b.Height(b.Nearest(cellX, cellY, worldZ))
}

// NSWENearest returns the passability mask of the layer, at the given cell, closest to worldZ.
func (b *Multilayer) NSWENearest(cellX, cellY int, worldZ int32) NSWE {
	return b.NSWE(b.Nearest(cellX, cellY, worldZ))
}

// Nearest returns a handle to the layer, at the given cell, closest to
// worldZ. When two layers are equally close, the higher of the two wins.
func (b *Multilayer) Nearest(cellX, cellY int, worldZ int32) int {
	ci := cellIndex(cellX, cellY)
	layers := b.cells[ci]

	best := 0
	limit := int32(math.MaxInt32)
	for i, c := range layers {
		d := abs32(int32(c.Height) - worldZ)
		if d > limit {
			break
		}
		limit = d
		best = i
	}
	return ci*layerSlot + best
}

// Above returns a handle to the first layer above worldZ at the given
// cell, scanning from the topmost layer down, or -1 if none qualifies.
func (b *Multilayer) Above(cellX, cellY int, worldZ int32) int {
	ci := cellIndex(cellX, cellY)
	layers := b.cells[ci]
	for i := len(layers) - 1; i >= 0; i-- {
		if int32(layers[i].Height) > worldZ {
			return ci*layerSlot + i
		}
	}
	return -1
}

// Below returns a handle to the first layer below worldZ at the given
// cell, scanning from the bottommost layer up, or -1 if none qualifies.
func (b *Multilayer) Below(cellX, cellY int, worldZ int32) int {
	ci := cellIndex(cellX, cellY)
	layers := b.cells[ci]
	for i := 0; i < len(layers); i++ {
		if int32(layers[i].Height) < worldZ {
			return ci*layerSlot + i
		}
	}
	return -1
}

// Height resolves a layer handle, from Nearest, Above, or Below, to its height.
func (b *Multilayer) Height(layer int) int16 {
	ci, li := layer/layerSlot, layer%layerSlot
	return b.cells[ci][li].Height
}

// NSWE resolves a layer handle, from Nearest, Above, or Below, to its passability mask.
func (b *Multilayer) NSWE(layer int) NSWE {
	ci, li := layer/layerSlot, layer%layerSlot
	return b.cells[ci][li].NSWE
}

// Cells returns a copy of the local cell's stored layers, ordered from
// lowest to highest.
func (b *Multilayer) Cells(cellX, cellY int) []Cell {
	return append([]Cell(nil), b.cells[cellIndex(cellX, cellY)]...)
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}
