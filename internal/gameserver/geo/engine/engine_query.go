package engine

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
)

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
			checkX = min(max(int(float64(ox)+float64(checkY-oy)/m), gridX), gridX+block.CellSize-1)
			gridY += dir.stepY
			ny += dir.signumY
		}

		if !nswe.Allows(step) {
			return false
		}

		next := e.blockAtGeo(nx, ny)
		layer := next.Below(localCell(nx), localCell(ny), int32(goz+block.CellIgnoreHeight))
		if layer == -1 {
			return false
		}

		gox = nx
		goy = ny
		goz = int(next.Height(layer))
		nswe = next.NSWE(layer)
	}

	return goz == int(e.Height(tx, ty, tz))
}

// CanMoveAround reports whether the 3x3 geodata cell block centered on the
// world position is fully open in every direction, matching Java's
// GeoEngine.canMoveAround: a single point can sit on open ground yet still
// be enclosed by blocked edges on every side, so a walkability check must
// look at the surrounding cells rather than the cell alone.
func (e *Engine) CanMoveAround(worldX, worldY, worldZ int) bool {
	geoX := GeoX(worldX)
	geoY := GeoY(worldY)

	for ix := -1; ix <= 1; ix++ {
		for iy := -1; iy <= 1; iy++ {
			if e.nsweNearest(geoX+ix, geoY+iy, worldZ) != block.AllDirections {
				return false
			}
		}
	}

	return true
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

// CanSeeActor reports whether two actors, each supplying its own collision
// height, share mutual line of sight. Collision heights are converted to
// eye height via SightHeight before the query.
func (e *Engine) CanSeeActor(ox, oy, oz int, oCollisionHeight float64, tx, ty, tz int, tCollisionHeight float64) bool {
	return e.CanSeeWithHeights(ox, oy, oz, e.SightHeight(oCollisionHeight), tx, ty, tz, e.SightHeight(tCollisionHeight))
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
	if layer == -1 {
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
			checkX = min(max(int(float64(ox)+float64(checkY-oy)/m), gridX), gridX+block.CellSize-1)
			gridY += dir.stepY
			goy += dir.signumY
		}

		current = e.blockAtGeo(gox, goy)
		losZ := float64(oz) + oheight + float64(e.maxObstacleHeight)
		losZ += mz * math.Sqrt(float64((checkX-ox)*(checkX-ox)+(checkY-oy)*(checkY-oy)))

		if nswe.Allows(step) {
			layer = current.Below(localCell(gox), localCell(goy), int32(groundZ+block.CellIgnoreHeight))
		} else {
			layer = current.Above(localCell(gox), localCell(goy), int32(groundZ-2*block.CellHeight))
		}
		if layer == -1 {
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
