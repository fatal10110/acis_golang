package dynamic

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

const (
	worldXMin = geo.WorldXMin
	worldYMin = geo.WorldYMin
)

// Sampler provides the static geodata lookups door shaping needs.
type Sampler interface {
	HeightNearest(geoX, geoY, worldZ int) int16
	Above(geoX, geoY, worldZ int) (int16, bool)
}

// NewDoorObject converts one static door template into a toggleable
// geodata object by scanning a grid of sample points around the door's
// polygon footprint and marking which cells fall inside it.
func NewDoorObject(tmpl *door.Template, sampler Sampler) (Object, error) {
	if tmpl == nil {
		return nil, fmt.Errorf("geo/dynamic: nil door template")
	}
	if sampler == nil {
		return nil, fmt.Errorf("geo/dynamic: nil sampler")
	}

	triangles, err := triangulate(tmpl.Coordinates)
	if err != nil {
		return nil, fmt.Errorf("geo/dynamic: door %d: %w", tmpl.ID, err)
	}

	minX, maxX, minY, maxY := bounds(tmpl.Coordinates)
	x := geoX(minX) - 1
	y := geoY(minY) - 1
	sizeX := (geoX(maxX) + 1) - x + 1
	sizeY := (geoY(maxY) + 1) - y + 1
	originX := geoX(tmpl.Position.X)
	originY := geoY(tmpl.Position.Y)
	originZ := int(sampler.HeightNearest(originX, originY, tmpl.Position.Z))
	height := tmpl.Height
	if above, ok := sampler.Above(originX, originY, originZ); ok {
		layerDiff := int(above) - originZ
		if height > layerDiff {
			height = layerDiff - block.CellIgnoreHeight
		}
	}

	limit := block.CellIgnoreHeight
	if tmpl.Kind == door.KindWall {
		limit *= 4
	}
	inside := make([][]bool, sizeX)
	for ix := range inside {
		inside[ix] = make([]bool, sizeY)
		for iy := range inside[ix] {
			gx := x + ix
			gy := y + iy
			z := int(sampler.HeightNearest(gx, gy, tmpl.Position.Z))
			if absInt(z-tmpl.Position.Z) > limit {
				continue
			}
			wx := worldX(gx)
			wy := worldY(gy)
		cell:
			for sx := wx - 6; sx <= wx+6; sx += 2 {
				for sy := wy - 6; sy <= wy+6; sy += 2 {
					if polygonContains(triangles, sx, sy) {
						inside[ix][iy] = true
						break cell
					}
				}
			}
		}
	}

	return &object{
		geoX:   x,
		geoY:   y,
		geoZ:   originZ,
		height: height,
		data:   CalculateGeoObject(inside),
	}, nil
}

func bounds(points []location.Point) (minX, maxX, minY, maxY int) {
	minX, minY = points[0].X, points[0].Y
	maxX, maxY = minX, minY
	for _, p := range points[1:] {
		if p.X < minX {
			minX = p.X
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}
	return
}

// triangle mirrors the aCis reference's Triangle: it stores point A and the
// BA/CA edge vectors so isInside can run its exact-integer half-plane test.
type triangle struct {
	ax, ay   int
	bax, bay int
	cax, cay int
}

func newTriangle(a, b, c location.Point) triangle {
	return triangle{
		ax: a.X, ay: a.Y,
		bax: b.X - a.X, bay: b.Y - a.Y,
		cax: c.X - a.X, cay: c.Y - a.Y,
	}
}

// isInside reports whether (x, y) lies on the same side of all three edges,
// edges inclusive. Products use int64 to match the reference's long math.
func (t triangle) isInside(x, y int) bool {
	dx := int64(x - t.ax)
	dy := int64(y - t.ay)
	bax, bay := int64(t.bax), int64(t.bay)
	cax, cay := int64(t.cax), int64(t.cay)

	a := (0-dx)*(bay-0)-(bax-0)*(0-dy) >= 0
	b := (bax-dx)*(cay-bay)-(cax-bax)*(bay-dy) >= 0
	c := (cax-dx)*(0-cay)-(0-cax)*(cay-dy) >= 0
	return a == b && b == c
}

// polygonContains reports whether (x, y) falls inside any triangle of a
// triangulated polygon, matching the reference's Polygon.isInside.
func polygonContains(triangles []triangle, x, y int) bool {
	for _, t := range triangles {
		if t.isInside(x, y) {
			return true
		}
	}
	return false
}

const triangulationMaxLoops = 100

// triangulate ear-clips poly into triangles using Kong's algorithm, matching
// the reference's Kong.doTriangulation. poly must be a simple, monotone
// polygon; anything else fails after triangulationMaxLoops ear-search passes.
func triangulate(poly []location.Point) ([]triangle, error) {
	if len(poly) < 3 {
		return nil, fmt.Errorf("geo/dynamic: polygon needs at least 3 points, got %d", len(poly))
	}

	points := append([]location.Point(nil), poly...)
	isCw := polygonOrientation(points)
	nonConvex := nonConvexPoints(points, isCw)
	return triangulationAlgorithm(points, isCw, nonConvex)
}

func nextIndex(size, index int) int {
	if index++; index >= size {
		return index - size
	}
	return index
}

func prevIndex(size, index int) int {
	if index--; index < 0 {
		return size + index
	}
	return index
}

func polygonOrientation(points []location.Point) bool {
	size := len(points)
	index := 0
	point := points[0]
	for i := 1; i < size; i++ {
		pt := points[i]
		if pt.X < point.X || (pt.X == point.X && pt.Y > point.Y) {
			point = pt
			index = i
		}
	}
	prev := points[prevIndex(size, index)]
	next := points[nextIndex(size, index)]
	vx := point.X - prev.X
	vy := point.Y - prev.Y
	res := next.X*vy - next.Y*vx + vx*prev.Y - vy*prev.X
	return res <= 0
}

func nonConvexPoints(points []location.Point, isCw bool) []location.Point {
	size := len(points)
	var out []location.Point
	for i := 0; i < size-1; i++ {
		point := points[i]
		next := points[nextIndex(size, i)]
		nextNext := points[nextIndex(size, i+1)]
		vx := next.X - point.X
		vy := next.Y - point.Y
		res := (nextNext.X*vy - nextNext.Y*vx + vx*point.Y - vy*point.X) > 0
		if res == isCw {
			out = append(out, next)
		}
	}
	return out
}

func isConvex(isCw bool, a, b, c location.Point) bool {
	bax := b.X - a.X
	bay := b.Y - a.Y
	cw := (c.X*bay - c.Y*bax + bax*a.Y - bay*a.X) > 0
	return cw != isCw
}

// kongPointInTriangle is Kong's own ear-detection containment test: strict,
// float-division barycentric coefficients, distinct from triangle.isInside.
func kongPointInTriangle(a, b, c, p location.Point) bool {
	bax := float64(b.X - a.X)
	bay := float64(b.Y - a.Y)
	cax := float64(c.X - a.X)
	cay := float64(c.Y - a.Y)
	pax := float64(p.X - a.X)
	pay := float64(p.Y - a.Y)

	det := bax*cay - cax*bay
	ba := (bax*pay - pax*bay) / det
	ca := (pax*cay - cax*pay) / det
	return ba > 0 && ca > 0 && (ba+ca) < 1
}

func isEar(isCw bool, nonConvex []location.Point, a, b, c location.Point) bool {
	if !isConvex(isCw, a, b, c) {
		return false
	}
	for _, p := range nonConvex {
		if kongPointInTriangle(a, b, c, p) {
			return false
		}
	}
	return true
}

func triangulationAlgorithm(points []location.Point, isCw bool, nonConvex []location.Point) ([]triangle, error) {
	var triangles []triangle
	size := len(points)
	index := 1
	for loops := 0; size > 3; loops++ {
		if loops == triangulationMaxLoops {
			return nil, fmt.Errorf("geo/dynamic: coordinates do not form a monotone polygon")
		}

		indexPrev := prevIndex(size, index)
		indexNext := nextIndex(size, index)
		pPrev := points[indexPrev]
		p := points[index]
		pNext := points[indexNext]

		if isEar(isCw, nonConvex, pPrev, p, pNext) {
			triangles = append(triangles, newTriangle(pPrev, p, pNext))
			points = append(points[:index], points[index+1:]...)
			size--
			index = prevIndex(size, index)
		} else {
			index = indexNext
		}
	}
	triangles = append(triangles, newTriangle(points[0], points[1], points[2]))
	return triangles, nil
}

func geoX(worldX int) int { return (worldX - worldXMin) >> 4 }
func geoY(worldY int) int { return (worldY - worldYMin) >> 4 }
func worldX(geoX int) int { return (geoX << 4) + worldXMin + 8 }
func worldY(geoY int) int { return (geoY << 4) + worldYMin + 8 }
func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
