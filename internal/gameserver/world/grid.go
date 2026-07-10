package world

// World bounds, in game coordinates. Derived from the game's fixed 32768-unit
// tile size and the map's tile span (X 16..26, Y 10..25).
const (
	tileSize = 32768

	MinX = (16 - 20) * tileSize
	MaxX = (26-19)*tileSize - 1
	MinY = (10 - 18) * tileSize
	MaxY = (25-17)*tileSize - 1
)

// regionSize is the edge length, in game coordinates, of one grid region.
const regionSize = 2048

// RegionsX and RegionsY are the grid's dimensions, in regions.
const (
	RegionsX = (MaxX - MinX + 1) / regionSize
	RegionsY = (MaxY - MinY + 1) / regionSize
)

// Grid indexes the world into a fixed RegionsX by RegionsY array of Regions,
// built once and never resized.
type Grid struct {
	regions [RegionsX][RegionsY]*Region
}

// NewGrid returns a Grid with every Region constructed and ready to use.
func NewGrid() *Grid {
	g := &Grid{}
	for x := 0; x < RegionsX; x++ {
		for y := 0; y < RegionsY; y++ {
			g.regions[x][y] = newRegion(x, y)
		}
	}
	return g
}

// RegionAt returns the Region containing game coordinate (x, y). ok is false
// if the coordinate falls outside the world's bounds.
func (g *Grid) RegionAt(x, y int) (region *Region, ok bool) {
	if x < MinX || x > MaxX || y < MinY || y > MaxY {
		return nil, false
	}
	return g.regions[(x-MinX)/regionSize][(y-MinY)/regionSize], true
}

// Neighbors returns every Region within depth grid steps of r, including r
// itself, clipped to the grid's edges.
func (g *Grid) Neighbors(r *Region, depth int) []*Region {
	var out []*Region
	for ix := -depth; ix <= depth; ix++ {
		x := r.tileX + ix
		if x < 0 || x >= RegionsX {
			continue
		}
		for iy := -depth; iy <= depth; iy++ {
			y := r.tileY + iy
			if y < 0 || y >= RegionsY {
				continue
			}
			out = append(out, g.regions[x][y])
		}
	}
	return out
}
