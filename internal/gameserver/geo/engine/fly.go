package engine

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// CanFly reports whether a flier oheight tall can travel the straight 3D line
// from (ox,oy,oz) to (tx,ty,tz): every cell the line crosses must keep its
// floor at or below the line and its ceiling at or above the line plus
// oheight. A target outside the world never is reachable.
func (e *Engine) CanFly(ox, oy, oz int, oheight float64, tx, ty, tz int) bool {
	if OutOfWorld(tx, ty) {
		return false
	}
	_, ok := e.flyLine(ox, oy, oz, oheight, tx, ty, tz, false, nil)
	return ok
}

// ValidFlyLocation returns the last point a flier oheight tall reaches on the
// straight 3D line from (ox,oy,oz) toward (tx,ty,tz): the border of the last
// cell it clears, at the line's height there, or the target itself when the
// whole line is clear. The walk stops at the border of the geodata grid too.
func (e *Engine) ValidFlyLocation(ox, oy, oz int, oheight float64, tx, ty, tz int) location.Location {
	return e.ValidFlyLocationTraced(ox, oy, oz, oheight, tx, ty, tz, nil)
}

// ValidFlyLocationTraced is ValidFlyLocation recording into tr both
// endpoint cells in blue 32 below their height, each border crossed with
// the cell's corridor bottom and top in green, and the cell the flight
// stops on in red.
func (e *Engine) ValidFlyLocationTraced(ox, oy, oz int, oheight float64, tx, ty, tz int, tr *Trace) location.Location {
	stop, _ := e.flyLine(ox, oy, oz, oheight, tx, ty, tz, true, tr)
	return stop
}

// flyLine walks the cells the line from (ox,oy,oz) to (tx,ty,tz) crosses,
// checking each against the flier's corridor: the line's height at the cell
// border (the bottom) and that plus oheight (the top). It reports the point
// the walk stopped at and whether it ran the whole line. bounded stops the
// walk at the geodata grid border, a point at the last geodata height read.
func (e *Engine) flyLine(ox, oy, oz int, oheight float64, tx, ty, tz int, bounded bool, tr *Trace) (location.Location, bool) {
	gox := GeoX(ox)
	goy := GeoY(oy)
	goz := int(e.heightNearest(gox, goy, oz))
	gtx := GeoX(tx)
	gty := GeoY(ty)

	dx := tx - ox
	dy := ty - oy
	dz := tz - oz
	m := float64(dy) / float64(dx)
	// The squared deltas are summed in 32-bit int before the square root, so
	// a line longer than ~46340 units in one axis wraps (a negative sum makes
	// the slope NaN); kept as observable behavior.
	mz := float64(dz) / math.Sqrt(float64(int32(dx)*int32(dx)+int32(dy)*int32(dy)))
	dir := moveDirectionFor(gtx-gox, gty-goy)
	gridX := alignCell(ox)
	gridY := alignCell(oy)
	height := int32(commons.TruncInt32Sat(oheight))
	maxGeoX := GeoX(WorldXMax)
	maxGeoY := GeoY(WorldYMax)
	tr.cellSquare(TraceBlue, gridX, gridY, goz-flyTraceDrop)
	tr.cellSquare(TraceBlue, alignCell(tx), alignCell(ty), tz-flyTraceDrop)

	checkZ := oz
	for gox != gtx || goy != gty {
		checkX := gridX + dir.offsetX
		checkY := int(float64(oy) + m*float64(checkX-ox))

		inX := dir.stepX != 0 && GeoY(checkY) == goy
		if inX {
			gridX += dir.stepX
			gox += dir.signumX
		} else {
			checkY = gridY + dir.offsetY
			checkX = min(max(int(float64(ox)+float64(checkY-oy)/m), gridX), gridX+block.CellSize-1)
			gridY += dir.stepY
			goy += dir.signumY
		}

		if bounded && (gox < 0 || gox > maxGeoX || goy < 0 || goy > maxGeoY) {
			tr.cellSquare(TraceRed, gridX, gridY, goz)
			return location.Location{X: checkX, Y: checkY, Z: goz}, false
		}
		stopped := location.Location{X: checkX, Y: checkY, Z: checkZ}

		next := e.blockAtGeo(gox, goy)
		stepDX := int32(checkX - ox)
		stepDY := int32(checkY - oy)
		bottomZ := int32(oz) + commons.TruncInt32Sat(mz*math.Sqrt(float64(stepDX*stepDX+stepDY*stepDY)))

		// Some geodata always lies below a flier: the highest layer under
		// the corridor top must stay at or under its bottom.
		layer, cell := next.belowCell(localCell(gox), localCell(goy), bottomZ+height, nil)
		if layer == -1 {
			tr.cellSquare(TraceRed, gridX, gridY, int(bottomZ)-flyTraceDrop)
			return stopped, false
		}
		goz = int(cell.Height)
		if int32(goz) > bottomZ {
			tr.cellSquare(TraceRed, gridX, gridY, int(bottomZ)-flyTraceDrop)
			return stopped, false
		}
		tr.border(dir, inX, checkX, checkY, int(bottomZ)-flyTraceDrop)
		tr.cellSquare(TraceGreen, gridX, gridY, int(bottomZ)-flyTraceDrop)

		// The lowest layer over the corridor bottom must clear its top.
		topZ := bottomZ + height
		if layer, cell = next.aboveCell(localCell(gox), localCell(goy), bottomZ, nil); layer != -1 {
			goz = int(cell.Height)
			if int32(goz) < topZ {
				tr.cellSquare(TraceRed, gridX, gridY, int(topZ)-flyTraceDrop)
				return stopped, false
			}
		}
		tr.cellSquare(TraceGreen, gridX, gridY, int(topZ)-flyTraceDrop)

		checkZ = int(bottomZ)
	}

	return location.Location{X: tx, Y: ty, Z: tz}, true
}

// flyTraceDrop is how far below the corridor a fly trace draws its cells.
const flyTraceDrop = 32
