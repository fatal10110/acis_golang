package block

import "fmt"

// Kind identifies which concrete layout implements a Block.
type Kind int

const (
	KindNull Kind = iota
	KindFlat
	KindComplex
	KindMultilayer
)

// String returns Kind's name, or "Kind(N)" for a value outside the
// defined constants.
func (k Kind) String() string {
	switch k {
	case KindNull:
		return "null"
	case KindFlat:
		return "flat"
	case KindComplex:
		return "complex"
	case KindMultilayer:
		return "multilayer"
	default:
		return fmt.Sprintf("Kind(%d)", int(k))
	}
}

// Block answers height, passability, and layer queries for one block: an
// 8x8 grid of geodata cells (CellsX x CellsY), each CellSize world units
// wide. Cell coordinates given to its methods are local to the block, in
// [0, CellsX) and [0, CellsY); resolving a world or region position to a
// block and its local cell is the caller's responsibility. An
// out-of-range cell coordinate panics, the same as any other
// out-of-bounds slice access.
//
// Nearest, Above, and Below each return an opaque handle to the matching
// layer (Above and Below return -1 when no layer qualifies); Height and
// NSWE resolve a handle to that layer's data. The indirection lets a
// caller step from one floor to the next without recomputing which cell
// it is in.
type Block interface {
	// Kind identifies which concrete layout implements this Block.
	Kind() Kind

	// HasGeodata reports whether the block carries real geodata; it is
	// false only for a placeholder standing in for an unloaded region.
	HasGeodata() bool

	// Layers returns how many height layers the cell at (cellX, cellY) has.
	Layers(cellX, cellY int) int

	// HeightNearest returns the height of the layer, at the given cell,
	// closest to worldZ.
	HeightNearest(cellX, cellY int, worldZ int32) int16

	// NSWENearest returns the passability mask of the layer, at the given
	// cell, closest to worldZ.
	NSWENearest(cellX, cellY int, worldZ int32) NSWE

	// Nearest returns a handle to the layer, at the given cell, closest to worldZ.
	Nearest(cellX, cellY int, worldZ int32) int

	// Above returns a handle to the first layer above worldZ at the given
	// cell, or -1 if the cell has none.
	Above(cellX, cellY int, worldZ int32) int

	// Below returns a handle to the first layer below worldZ at the given
	// cell, or -1 if the cell has none.
	Below(cellX, cellY int, worldZ int32) int

	// Height resolves a layer handle, from Nearest, Above, or Below, to its height.
	Height(layer int) int16

	// NSWE resolves a layer handle, from Nearest, Above, or Below, to its passability mask.
	NSWE(layer int) NSWE
}
