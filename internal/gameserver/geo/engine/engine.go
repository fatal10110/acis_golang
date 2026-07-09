package engine

import (
	"fmt"
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
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

	// ponytail: keep the shipped default until geoengine.properties lands.
	maxObstacleHeight = 32
)

// Engine serves geodata height, movement, and line-of-sight queries.
type Engine struct {
	regions [regionTilesX][regionTilesY]*block.Region
}

// New returns an empty engine that answers unloaded regions with null geodata.
func New() *Engine {
	return &Engine{}
}

// SetRegion installs one decoded geodata region at the given tile coordinates.
func (e *Engine) SetRegion(regionX, regionY int, region *block.Region) error {
	if regionX < TileXMin || regionX > TileXMax || regionY < TileYMin || regionY > TileYMax {
		return fmt.Errorf("geo/engine: region %d_%d out of range", regionX, regionY)
	}
	if region == nil {
		return fmt.Errorf("geo/engine: region %d_%d is nil", regionX, regionY)
	}
	e.regions[regionX-TileXMin][regionY-TileYMin] = region
	return nil
}

// HasGeo reports whether the world position belongs to a loaded non-null block.
func (e *Engine) HasGeo(worldX, worldY int) bool {
	return e.blockAtGeo(GeoX(worldX), GeoY(worldY)).HasGeodata()
}

// Height returns the geodata height nearest to the given world position.
func (e *Engine) Height(worldX, worldY, worldZ int) int16 {
	return e.heightNearest(GeoX(worldX), GeoY(worldY), worldZ)
}

// CanMove reports whether a straight move from origin to target crosses no blocked edge
// and stays on a reachable floor.
func (e *Engine) CanMove(ox, oy, oz, tx, ty, tz int) bool {
	if OutOfWorld(tx, ty) {
		return false
	}

	gox := GeoX(ox)
	goy := GeoY(oy)
	goz := int(e.heightNearest(gox, goy, oz))
	gtx := GeoX(tx)
	gty := GeoY(ty)

	if gox == gtx && goy == gty {
		return goz == int(e.Height(tx, ty, tz))
	}

	nswe := e.nsweNearest(gox, goy, goz)
	m := float64(ty-oy) / float64(tx-ox)
	dir := moveDirectionFor(gtx-gox, gty-goy)
	gridX := alignCell(ox)
	gridY := alignCell(oy)

	nx := gox
	ny := goy
	for gox != gtx || goy != gty {
		checkX := gridX + dir.offsetX
		checkY := int(float64(oy) + m*float64(checkX-ox))

		step := dir.dirY
		if dir.stepX != 0 && GeoY(checkY) == goy {
			gridX += dir.stepX
			nx += dir.signumX
			step = dir.dirX
		} else {
			checkY = gridY + dir.offsetY
			checkX = clamp(int(float64(ox)+float64(checkY-oy)/m), gridX, gridX+block.CellSize-1)
			gridY += dir.stepY
			ny += dir.signumY
		}

		if !nswe.Allows(step) {
			return false
		}

		next := e.blockAtGeo(nx, ny)
		layer := next.Below(localCell(nx), localCell(ny), int32(goz+block.CellIgnoreHeight))
		if layer < 0 {
			return false
		}

		gox = nx
		goy = ny
		goz = int(next.Height(layer))
		nswe = next.NSWE(layer)
	}

	return goz == int(e.Height(tx, ty, tz))
}

// CanSee reports whether the two world positions share mutual line of sight.
func (e *Engine) CanSee(ox, oy, oz, tx, ty, tz int) bool {
	return e.CanSeeWithHeights(ox, oy, oz, 0, tx, ty, tz, 0)
}

// CanSeeWithHeights reports whether the two world positions share mutual line of sight
// when each endpoint is elevated by the given collision height.
func (e *Engine) CanSeeWithHeights(ox, oy, oz int, oheight float64, tx, ty, tz int, theight float64) bool {
	return e.canSee(ox, oy, oz, oheight, tx, ty, tz, theight) &&
		e.canSee(tx, ty, tz, theight, ox, oy, oz, oheight)
}

// GeoX converts a world X coordinate to geodata X.
func GeoX(worldX int) int {
	return (worldX - WorldXMin) >> 4
}

// GeoY converts a world Y coordinate to geodata Y.
func GeoY(worldY int) int {
	return (worldY - WorldYMin) >> 4
}

// WorldX converts a geodata X coordinate to the world-space cell center.
func WorldX(geoX int) int {
	return (geoX << 4) + WorldXMin + 8
}

// WorldY converts a geodata Y coordinate to the world-space cell center.
func WorldY(geoY int) int {
	return (geoY << 4) + WorldYMin + 8
}

// OutOfWorld reports whether the world position lies outside the supported map.
func OutOfWorld(worldX, worldY int) bool {
	return worldX < WorldXMin || worldX > WorldXMax || worldY < WorldYMin || worldY > WorldYMax
}

func (e *Engine) canSee(ox, oy, oz int, oheight float64, tx, ty, tz int, theight float64) bool {
	if OutOfWorld(ox, oy) || OutOfWorld(tx, ty) {
		return false
	}

	gox := GeoX(ox)
	goy := GeoY(oy)
	gtx := GeoX(tx)
	gty := GeoY(ty)

	current := e.blockAtGeo(gox, goy)
	layer := current.Below(localCell(gox), localCell(goy), int32(oz+block.CellHeight))
	if layer < 0 {
		return false
	}
	if gox == gtx && goy == gty {
		return layer == current.Below(localCell(gtx), localCell(gty), int32(tz+block.CellHeight))
	}

	groundZ := int(current.Height(layer))
	nswe := current.NSWE(layer)
	dx := tx - ox
	dy := ty - oy
	m := float64(dy) / float64(dx)
	dz := float64(tz) + theight - (float64(oz) + oheight)
	mz := dz / math.Sqrt(float64(dx*dx+dy*dy))
	dir := moveDirectionFor(gtx-gox, gty-goy)
	gridX := alignCell(ox)
	gridY := alignCell(oy)

	for gox != gtx || goy != gty {
		checkX := gridX + dir.offsetX
		checkY := int(float64(oy) + m*float64(checkX-ox))

		step := dir.dirY
		if dir.stepX != 0 && GeoY(checkY) == goy {
			gridX += dir.stepX
			gox += dir.signumX
			step = dir.dirX
		} else {
			checkY = gridY + dir.offsetY
			checkX = clamp(int(float64(ox)+float64(checkY-oy)/m), gridX, gridX+block.CellSize-1)
			gridY += dir.stepY
			goy += dir.signumY
		}

		current = e.blockAtGeo(gox, goy)
		losZ := float64(oz) + oheight + maxObstacleHeight
		losZ += mz * math.Sqrt(float64((checkX-ox)*(checkX-ox)+(checkY-oy)*(checkY-oy)))

		if nswe.Allows(step) {
			layer = current.Below(localCell(gox), localCell(goy), int32(groundZ+block.CellIgnoreHeight))
		} else {
			layer = current.Above(localCell(gox), localCell(goy), int32(groundZ-2*block.CellHeight))
		}
		if layer < 0 {
			return false
		}

		nextZ := int(current.Height(layer))
		if float64(nextZ) > losZ {
			return false
		}

		groundZ = nextZ
		nswe = current.NSWE(layer)
	}

	return true
}

func (e *Engine) heightNearest(geoX, geoY, worldZ int) int16 {
	return e.blockAtGeo(geoX, geoY).HeightNearest(localCell(geoX), localCell(geoY), int32(worldZ))
}

func (e *Engine) nsweNearest(geoX, geoY, worldZ int) block.NSWE {
	return e.blockAtGeo(geoX, geoY).NSWENearest(localCell(geoX), localCell(geoY), int32(worldZ))
}

func (e *Engine) blockAtGeo(geoX, geoY int) regionBlock {
	regionX := TileXMin + geoX/regionCellsX
	regionY := TileYMin + geoY/regionCellsY
	if geoX < 0 || geoY < 0 || regionX < TileXMin || regionX > TileXMax || regionY < TileYMin || regionY > TileYMax {
		return regionBlock{}
	}
	region := e.regions[regionX-TileXMin][regionY-TileYMin]
	if region == nil {
		return regionBlock{}
	}

	localGeoX := geoX % regionCellsX
	localGeoY := geoY % regionCellsY
	blockX := localGeoX / block.CellsX
	blockY := localGeoY / block.CellsY
	return regionBlock{region: region, blockX: blockX, blockY: blockY}
}

type regionBlock struct {
	region *block.Region
	blockX int
	blockY int
}

func (b regionBlock) HasGeodata() bool {
	return b.region != nil && b.region.HasGeodata(b.blockX, b.blockY)
}

func (b regionBlock) HeightNearest(cellX, cellY int, worldZ int32) int16 {
	if b.region == nil {
		return block.NullHeight(worldZ)
	}
	return b.region.HeightNearest(b.blockX, b.blockY, cellX, cellY, worldZ)
}

func (b regionBlock) NSWENearest(cellX, cellY int, worldZ int32) block.NSWE {
	if b.region == nil {
		return block.AllDirections
	}
	return b.region.NSWENearest(b.blockX, b.blockY, cellX, cellY, worldZ)
}

func (b regionBlock) Above(cellX, cellY int, worldZ int32) int {
	if b.region == nil {
		return 0
	}
	return b.region.Above(b.blockX, b.blockY, cellX, cellY, worldZ)
}

func (b regionBlock) Below(cellX, cellY int, worldZ int32) int {
	if b.region == nil {
		return 0
	}
	return b.region.Below(b.blockX, b.blockY, cellX, cellY, worldZ)
}

func (b regionBlock) Height(layer int) int16 {
	if b.region == nil {
		return 0
	}
	return b.region.Height(b.blockX, b.blockY, layer)
}

func (b regionBlock) NSWE(layer int) block.NSWE {
	if b.region == nil {
		return block.AllDirections
	}
	return b.region.NSWE(b.blockX, b.blockY, layer)
}

func localCell(geo int) int {
	return geo % block.CellsX
}

func alignCell(world int) int {
	return world &^ (block.CellSize - 1)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

type moveDirection struct {
	stepX   int
	stepY   int
	signumX int
	signumY int
	offsetX int
	offsetY int
	dirX    block.NSWE
	dirY    block.NSWE
}

func moveDirectionFor(gdx, gdy int) moveDirection {
	signumX := cmp(gdx)
	signumY := cmp(gdy)
	return moveDirection{
		stepX:   signumX * block.CellSize,
		stepY:   signumY * block.CellSize,
		signumX: signumX,
		signumY: signumY,
		offsetX: ternary(signumX >= 0, block.CellSize-1, 0),
		offsetY: ternary(signumY >= 0, block.CellSize-1, 0),
		dirX:    directionFlag(signumX, block.West, block.East),
		dirY:    directionFlag(signumY, block.North, block.South),
	}
}

func cmp(v int) int {
	switch {
	case v < 0:
		return -1
	case v > 0:
		return 1
	default:
		return 0
	}
}

func directionFlag(signum int, negative, positive block.NSWE) block.NSWE {
	switch {
	case signum < 0:
		return negative
	case signum > 0:
		return positive
	default:
		return 0
	}
}

func ternary(ok bool, yes, no int) int {
	if ok {
		return yes
	}
	return no
}
