package block

// CellSize is the width and depth of one geodata cell, in world units.
const CellSize = 16

// CellHeight is the vertical resolution, in world units, that geodata
// height values are quantized to and compared at.
const CellHeight = 8

// CellIgnoreHeight is the maximum vertical gap, in world units, that a
// caller should still treat as the same walkable layer rather than a
// distinct one when stepping from one cell to the next. It is six
// CellHeight units.
const CellIgnoreHeight = CellHeight * 6

// CellsX and CellsY are the dimensions of a block's cell grid: 8x8 cells.
// CellCount is the total number of cells in a block.
const (
	CellsX    = 8
	CellsY    = 8
	CellCount = CellsX * CellsY
)

// RegionBlocksX and RegionBlocksY are the dimensions of a geodata region
// in blocks: 256x256. RegionBlockCount is the total number of blocks in
// a region.
const (
	RegionBlocksX    = 256
	RegionBlocksY    = 256
	RegionBlockCount = RegionBlocksX * RegionBlocksY
)

// MaxLayers is the most height layers a single Multilayer cell may hold.
const MaxLayers = 127

// cellIndex returns the storage-order index of the cell at (cellX, cellY)
// within a block's CellsX x CellsY grid.
func cellIndex(cellX, cellY int) int {
	return cellX*CellsY + cellY
}
