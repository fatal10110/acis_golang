package block

import "math"

var _ Block = (*Null)(nil)

// Null is a placeholder block standing in for a region that carries no
// geodata (not loaded, or intentionally disabled): open and passable at
// any height, and treats whatever Z coordinate is queried as valid
// ground rather than reporting a stored one.
type Null struct{}

// Kind identifies b as KindNull.
func (b *Null) Kind() Kind { return KindNull }

// HasGeodata always reports false.
func (b *Null) HasGeodata() bool { return false }

// Layers always returns 1.
func (b *Null) Layers(cellX, cellY int) int { return 1 }

// HeightNearest returns worldZ unchanged, clamped to the int16 range every
// real stored height uses: with no geodata to consult, the queried height
// is assumed to already be valid ground.
func (b *Null) HeightNearest(cellX, cellY int, worldZ int32) int16 {
	return NullHeight(worldZ)
}

// NullHeight returns worldZ clamped to the int16 range used by stored
// geodata heights.
func NullHeight(worldZ int32) int16 {
	switch {
	case worldZ > math.MaxInt16:
		return math.MaxInt16
	case worldZ < math.MinInt16:
		return math.MinInt16
	default:
		return int16(worldZ)
	}
}

// NSWENearest always returns AllDirections.
func (b *Null) NSWENearest(cellX, cellY int, worldZ int32) NSWE { return AllDirections }

// Nearest always returns the layer handle 0.
func (b *Null) Nearest(cellX, cellY int, worldZ int32) int { return 0 }

// Above always returns the layer handle 0.
func (b *Null) Above(cellX, cellY int, worldZ int32) int { return 0 }

// Below always returns the layer handle 0.
func (b *Null) Below(cellX, cellY int, worldZ int32) int { return 0 }

// Height always returns 0, regardless of layer handle.
func (b *Null) Height(layer int) int16 { return 0 }

// NSWE always returns AllDirections, regardless of layer handle.
func (b *Null) NSWE(layer int) NSWE { return AllDirections }

// Cells returns a single nominal open layer at height 0. Unlike
// HeightNearest, which answers relative to the queried worldZ, this has
// no query to answer relative to — it exists only so a caller building
// its own overlay on top of a Null block (which by definition has no
// real geodata) has some baseline layer to start from.
func (b *Null) Cells(cellX, cellY int) []Cell {
	return []Cell{{Height: 0, NSWE: AllDirections}}
}
