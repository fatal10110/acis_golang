package block

var _ Block = (*Flat)(nil)

// Flat is a block whose entire cell grid shares one height and is fully
// passable in every direction: the simplest, most common geodata layout
// (open outdoor terrain with no overhangs).
type Flat struct {
	height int16
}

// NewFlat returns a Flat block at the given height.
func NewFlat(height int16) *Flat {
	return &Flat{height: height}
}

// Kind identifies b as KindFlat.
func (b *Flat) Kind() Kind { return KindFlat }

// HasGeodata always reports true.
func (b *Flat) HasGeodata() bool { return true }

// Layers always returns 1: a Flat block has a single height layer everywhere.
func (b *Flat) Layers(cellX, cellY int) int { return 1 }

// HeightNearest returns the block's single height, regardless of cell or worldZ.
func (b *Flat) HeightNearest(cellX, cellY int, worldZ int32) int16 { return b.height }

// NSWENearest always returns AllDirections.
func (b *Flat) NSWENearest(cellX, cellY int, worldZ int32) NSWE { return AllDirections }

// Nearest always returns the block's single layer handle, 0.
func (b *Flat) Nearest(cellX, cellY int, worldZ int32) int { return 0 }

// Above returns the layer handle 0 when the block's height is above
// worldZ, or -1 otherwise.
func (b *Flat) Above(cellX, cellY int, worldZ int32) int {
	if int32(b.height) > worldZ {
		return 0
	}
	return -1
}

// Below returns the layer handle 0 when the block's height is below
// worldZ, or -1 otherwise.
func (b *Flat) Below(cellX, cellY int, worldZ int32) int {
	if int32(b.height) < worldZ {
		return 0
	}
	return -1
}

// Height returns the block's single height, regardless of layer handle.
func (b *Flat) Height(layer int) int16 { return b.height }

// NSWE always returns AllDirections, regardless of layer handle.
func (b *Flat) NSWE(layer int) NSWE { return AllDirections }

// Cells returns the block's single layer data for any local cell.
func (b *Flat) Cells(cellX, cellY int) []Cell {
	return []Cell{{Height: b.height, NSWE: AllDirections}}
}
