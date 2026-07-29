package engine

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/dynamic"
)

const (
	TileXMin = 16
	TileXMax = 26
	TileYMin = 10
	TileYMax = 25

	TileSize  = 32768
	WorldXMin = (TileXMin - 20) * TileSize
	WorldXMax = (TileXMax-19)*TileSize - 1
	WorldYMin = (TileYMin - 18) * TileSize
	WorldYMax = (TileYMax-17)*TileSize - 1

	regionCellsX = block.RegionBlocksX * block.CellsX
	regionCellsY = block.RegionBlocksY * block.CellsY
	regionTilesX = TileXMax - TileXMin + 1
	regionTilesY = TileYMax - TileYMin + 1
)

// Engine serves geodata height, movement, and line-of-sight queries.
//
// regions is written only by SetRegion during boot loading and never again
// afterward (LoadEngine finishes before the engine is handed to any
// concurrent reader), so query paths read it without synchronization.
//
// dynamicBlocks holds the door/fence overlay blocks; it changes only on the
// rare toggleObject write (a door opening/closing), against a read volume of
// every CanMove/CanSee/pathfind cell step. It's held behind an atomic
// pointer so steady-state reads are a single atomic load plus a plain map
// read, no lock: toggleObject clones-and-swaps the map when it introduces a
// block the map doesn't have yet, and mutates an already-present block's
// *dynamic.Block in place, under that block's own internal lock, without
// touching the map at all. dynamicBlocksMu only serializes concurrent
// writers against each other; no read path ever acquires it.
type Engine struct {
	regionsMu sync.Mutex
	regions   [regionTilesX][regionTilesY]*block.Region

	dynamicBlocksMu sync.Mutex
	dynamicBlocks   atomic.Pointer[map[blockKey]*dynamic.Block]

	maxObstacleHeight     int
	partOfCharacterHeight int
}

// New returns an empty engine that answers unloaded regions with null geodata.
func New(options ...Options) *Engine {
	opts := DefaultOptions()
	if len(options) > 0 {
		opts = options[0]
	}
	return &Engine{maxObstacleHeight: opts.MaxObstacleHeight, partOfCharacterHeight: opts.PartOfCharacterHeight}
}

// MaxObstacleHeight returns the line-of-sight obstacle allowance.
func (e *Engine) MaxObstacleHeight() int {
	return e.maxObstacleHeight
}

// SightHeight converts an actor's collision height into its line-of-sight
// eye height: real creature height is collision height * 2, of which the
// configured PartOfCharacterHeight percentage counts for visibility.
func (e *Engine) SightHeight(collisionHeight float64) float64 {
	return collisionHeight * 2 * float64(e.partOfCharacterHeight) / 100
}

// SetRegion installs one decoded geodata region at the given tile
// coordinates. Must only be called during boot, before any concurrent
// query — query paths read regions without synchronization.
func (e *Engine) SetRegion(regionX, regionY int, region *block.Region) error {
	if regionX < TileXMin || regionX > TileXMax || regionY < TileYMin || regionY > TileYMax {
		return fmt.Errorf("geo/engine: region %d_%d out of range", regionX, regionY)
	}
	if region == nil {
		return fmt.Errorf("geo/engine: region %d_%d is nil", regionX, regionY)
	}
	e.regionsMu.Lock()
	defer e.regionsMu.Unlock()
	e.regions[regionX-TileXMin][regionY-TileYMin] = region
	return nil
}

// AddObject applies obj's geodata changes to every loaded block it touches.
func (e *Engine) AddObject(obj dynamic.Object) {
	e.toggleObject(obj, true)
}

// RemoveObject removes obj's geodata changes from every dynamic block it touched.
func (e *Engine) RemoveObject(obj dynamic.Object) {
	e.toggleObject(obj, false)
}

// HasGeo reports whether the world position belongs to a loaded non-null block.
func (e *Engine) HasGeo(worldX, worldY int) bool {
	return e.blockAtGeo(GeoX(worldX), GeoY(worldY)).HasGeodata()
}

// Height returns the geodata height nearest to the given world position.
func (e *Engine) Height(worldX, worldY, worldZ int) int16 {
	return e.heightNearest(GeoX(worldX), GeoY(worldY), worldZ)
}

// HeightNearest returns the nearest height at geodata cell coordinates.
func (e *Engine) HeightNearest(geoX, geoY, worldZ int) int16 {
	return e.heightNearest(geoX, geoY, worldZ)
}

// NSWENearest returns the NSWE mask of the geodata layer nearest worldZ at
// geodata cell coordinates.
func (e *Engine) NSWENearest(geoX, geoY, worldZ int) block.NSWE {
	return e.nsweNearest(geoX, geoY, worldZ)
}

// NodeBelow returns the resolved height and own decoded NSWE mask of the
// geodata layer nearest at-or-below worldZ for the given geodata cell — the
// same getIndexBelow/getHeight/getNswe sequence pathfinding candidate
// generation needs to read a cell's own passability bits directly, instead
// of re-deriving them with a CanMove line-walk. ok is false only when a
// loaded region has no layer within reach of worldZ; an unloaded region
// always answers open, same as every other null-block query on this engine.
func (e *Engine) NodeBelow(geoX, geoY, worldZ int) (height int16, nswe block.NSWE, ok bool) {
	b := e.blockAtGeo(geoX, geoY)
	layer := b.Below(localCell(geoX), localCell(geoY), int32(worldZ))
	if layer == -1 {
		return 0, 0, false
	}
	return b.Height(layer), b.NSWE(layer), true
}

// NodeAtOrAbove returns the resolved height and own decoded NSWE mask of the
// first geodata layer at or above worldZ, scanning from the topmost layer
// down, whose height satisfies accept. This lets a caller reject a
// higher-but-invalid layer (e.g. one with no matching edge back to the
// searcher) and fall through to the next-highest qualifying layer, instead
// of being stuck with whichever layer happens to be topmost. ok is false if
// no layer both qualifies and satisfies accept.
func (e *Engine) NodeAtOrAbove(geoX, geoY, worldZ int, accept func(height int16) bool) (height int16, nswe block.NSWE, ok bool) {
	b := e.blockAtGeo(geoX, geoY)
	// ponytail: Cells() copies every layer at this cell; cold path (only
	// reached once per candidate that already missed NodeBelow's guess),
	// so the allocation is left as-is. Upgrade to a Layers()+per-index
	// accessor if profiling ever puts this on a hot path.
	cells := b.Cells(localCell(geoX), localCell(geoY))
	for i := len(cells) - 1; i >= 0; i-- {
		if int(cells[i].Height) >= worldZ && accept(cells[i].Height) {
			return cells[i].Height, cells[i].NSWE, true
		}
	}
	return 0, 0, false
}

// Above returns the first layer above worldZ at geodata cell coordinates.
func (e *Engine) Above(geoX, geoY, worldZ int) (int16, bool) {
	b := e.blockAtGeo(geoX, geoY)
	if !b.HasGeodata() {
		return 0, false
	}
	layer := b.Above(localCell(geoX), localCell(geoY), int32(worldZ))
	if layer == -1 {
		return 0, false
	}
	return b.Height(layer), true
}

// CanMove reports whether a straight move from origin to target crosses no blocked edge
// and stays on a reachable floor.
