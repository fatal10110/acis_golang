package engine

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// ValidLocation returns the last world-space point reachable along the straight
// line from (ox,oy,oz) toward (tx,ty,tz) before the route is blocked. When the
// full line is walkable and the target cell's floor matches the walked floor,
// the target itself is returned with its resolved geodata Z. When the actor
// cannot step off the origin cell (route closes immediately) or the target
// cell sits on a different floor, the origin is returned unchanged.
//
// Used as the no-path fallback for movement requests: rather than refusing to
// move at all when no pathfinder route exists, the actor advances as far as
// the terrain allows in the requested direction.
func (e *Engine) ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location {
	if OutOfWorld(tx, ty) {
		return location.Location{X: ox, Y: oy, Z: oz}
	}

	gox := GeoX(ox)
	goy := GeoY(oy)
	goz := int(e.heightNearest(gox, goy, oz))
	gtx := GeoX(tx)
	gty := GeoY(ty)
	gtz := int(e.heightNearest(gtx, gty, tz))

	if gox == gtx && goy == gty {
		if goz == int(e.Height(tx, ty, tz)) {
			return location.Location{X: tx, Y: ty, Z: gtz}
		}
		return location.Location{X: ox, Y: oy, Z: oz}
	}

	nswe := e.nsweNearest(gox, goy, goz)
	// tx == ox would have collapsed into the same-cell branch above, so the
	// slope is finite here. Float division mirrors the engine's CanMove line
	// walk, which produces identical stepping decisions.
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

		// A blocked edge or an unservable next cell ends the walk at this
		// border: the actor stops at the last grid boundary it reached.
		if !nswe.Allows(step) {
			return location.Location{X: checkX, Y: checkY, Z: goz}
		}
		next := e.blockAtGeo(nx, ny)
		layer := next.Below(localCell(nx), localCell(ny), int32(goz+block.CellIgnoreHeight))
		if layer == -1 {
			return location.Location{X: checkX, Y: checkY, Z: goz}
		}

		gox = nx
		goy = ny
		goz = int(next.Height(layer))
		nswe = next.NSWE(layer)
	}

	if goz == int(e.Height(tx, ty, tz)) {
		return location.Location{X: tx, Y: ty, Z: gtz}
	}
	return location.Location{X: ox, Y: oy, Z: oz}
}
