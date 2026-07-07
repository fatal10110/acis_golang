package block

var _ Block = (*Complex)(nil)

// Complex is a block where every cell carries its own independent height
// and passability mask, but still just one layer each (uneven ground
// with no overhangs, e.g. a slope or stairs).
type Complex struct {
	cells [CellCount]Cell
}

// NewComplex returns a Complex block built from the given per-cell data.
// cells is indexed by cellX*CellsY + cellY.
func NewComplex(cells [CellCount]Cell) *Complex {
	return &Complex{cells: cells}
}

// Kind identifies b as KindComplex.
func (b *Complex) Kind() Kind { return KindComplex }

// HasGeodata always reports true.
func (b *Complex) HasGeodata() bool { return true }

// Layers always returns 1: a Complex cell has a single height layer.
func (b *Complex) Layers(cellX, cellY int) int { return 1 }

// HeightNearest returns the given cell's height; worldZ is unused since
// the cell has only one layer.
func (b *Complex) HeightNearest(cellX, cellY int, worldZ int32) int16 {
	return b.cells[cellIndex(cellX, cellY)].Height
}

// NSWENearest returns the given cell's passability mask; worldZ is
// unused since the cell has only one layer.
func (b *Complex) NSWENearest(cellX, cellY int, worldZ int32) NSWE {
	return b.cells[cellIndex(cellX, cellY)].NSWE
}

// Nearest returns a handle to the given cell's single layer; worldZ is unused.
func (b *Complex) Nearest(cellX, cellY int, worldZ int32) int {
	return cellIndex(cellX, cellY)
}

// Above returns a handle to the given cell's layer when its height is
// above worldZ, or -1 otherwise.
func (b *Complex) Above(cellX, cellY int, worldZ int32) int {
	i := cellIndex(cellX, cellY)
	if int32(b.cells[i].Height) > worldZ {
		return i
	}
	return -1
}

// Below returns a handle to the given cell's layer when its height is
// below worldZ, or -1 otherwise.
func (b *Complex) Below(cellX, cellY int, worldZ int32) int {
	i := cellIndex(cellX, cellY)
	if int32(b.cells[i].Height) < worldZ {
		return i
	}
	return -1
}

// Height resolves a layer handle to its height.
func (b *Complex) Height(layer int) int16 { return b.cells[layer].Height }

// NSWE resolves a layer handle to its passability mask.
func (b *Complex) NSWE(layer int) NSWE { return b.cells[layer].NSWE }
