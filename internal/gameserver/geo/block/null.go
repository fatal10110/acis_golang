package block

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

// HeightNearest returns worldZ unchanged: with no geodata to consult,
// the queried height is assumed to already be valid ground.
func (b *Null) HeightNearest(cellX, cellY int, worldZ int32) int16 {
	return NullHeight(worldZ)
}

// NullHeight returns worldZ narrowed to the int16 range used by stored
// geodata heights by a truncating 16-bit conversion.
func NullHeight(worldZ int32) int16 { return int16(worldZ) }

// NSWENearest always returns AllDirections.
func (b *Null) NSWENearest(cellX, cellY int, worldZ int32) NSWE { return AllDirections }

// Nearest always returns the layer handle 0.
//
// Nearest, Above, and Below never answer -1 ("no layer"), whatever worldZ
// is: null geodata always offers one valid layer. This is the intended
// contract, not a missing height search.
//
// Null is the standalone form of the null-geodata contract. The engine does
// not query a Null value: its null answers come from Region's default
// branches (empty entries) and from the engine's unloaded-region block.
// Those paths mirror Null and must stay in sync with it.
func (b *Null) Nearest(cellX, cellY int, worldZ int32) int { return 0 }

// Above always returns the layer handle 0; see Nearest.
func (b *Null) Above(cellX, cellY int, worldZ int32) int { return 0 }

// Below always returns the layer handle 0; see Nearest.
func (b *Null) Below(cellX, cellY int, worldZ int32) int { return 0 }

// Height always returns 0, regardless of layer handle.
//
// This is deliberate and differs from HeightNearest on purpose: a query
// that resolves a layer handle first (Below then Height) reads ground at
// the constant height 0, not at the querying actor's Z. The engine's null
// paths (see Nearest) answer the same way, so a straight walk from loaded
// geodata into null geodata lands at height 0, and line of sight into it
// sees ground at 0. Do not change this to track the queried Z, here or in
// those mirrors; movement and sight results across null geodata depend on
// the constant.
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
