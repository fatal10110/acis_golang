package zone

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Form is the geometric footprint of a zone: a 3D volume that can answer
// point containment and 2D rectangle overlap (used to attach zones to the
// world's region grid). Forms are immutable once built.
type Form interface {
	// Contains reports whether the point (x, y, z) lies inside the volume.
	Contains(x, y, z int) bool
	// IntersectsRect reports whether the volume's 2D footprint overlaps the
	// axis-aligned rectangle spanning x1..x2 by y1..y2.
	IntersectsRect(x1, x2, y1, y2 int) bool
	// LowZ is the volume's lower z bound.
	LowZ() int
	// HighZ is the volume's upper z bound.
	HighZ() int
}

// relativeCCW classifies where the point (px, py) sits relative to the
// directed segment (x1, y1)->(x2, y2): -1 counter-clockwise, 1 clockwise,
// 0 on the segment (points beyond either endpoint on the segment's line
// report the side they would rotate toward). Double precision throughout;
// the exact branch structure is part of the intersection contract.
func relativeCCW(x1, y1, x2, y2, px, py float64) int {
	x2 -= x1
	y2 -= y1
	px -= x1
	py -= y1
	ccw := px*y2 - py*x2
	if ccw == 0.0 {
		// Collinear: decide by projection onto the segment direction.
		ccw = px*x2 + py*y2
		if ccw > 0.0 {
			px -= x2
			py -= y2
			ccw = px*x2 + py*y2
			if ccw < 0.0 {
				ccw = 0.0
			}
		}
	}
	switch {
	case ccw < 0.0:
		return -1
	case ccw > 0.0:
		return 1
	default:
		return 0
	}
}

// segmentsIntersect reports whether the closed segments a and b share at
// least one point (touching endpoints and collinear overlap both count).
func segmentsIntersect(ax1, ay1, ax2, ay2, bx1, by1, bx2, by2 int) bool {
	fa1x, fa1y, fa2x, fa2y := float64(ax1), float64(ay1), float64(ax2), float64(ay2)
	fb1x, fb1y, fb2x, fb2y := float64(bx1), float64(by1), float64(bx2), float64(by2)
	return relativeCCW(fa1x, fa1y, fa2x, fa2y, fb1x, fb1y)*relativeCCW(fa1x, fa1y, fa2x, fa2y, fb2x, fb2y) <= 0 &&
		relativeCCW(fb1x, fb1y, fb2x, fb2y, fa1x, fa1y)*relativeCCW(fb1x, fb1y, fb2x, fb2y, fa2x, fa2y) <= 0
}

// Cuboid is an axis-aligned box.
type Cuboid struct {
	x1, x2, y1, y2, z1, z2 int
}

// NewCuboid builds a Cuboid from two opposite corners; coordinates may be
// given in either order on each axis.
func NewCuboid(x1, x2, y1, y2, z1, z2 int) Cuboid {
	if x1 > x2 {
		x1, x2 = x2, x1
	}
	if y1 > y2 {
		y1, y2 = y2, y1
	}
	if z1 > z2 {
		z1, z2 = z2, z1
	}
	return Cuboid{x1: x1, x2: x2, y1: y1, y2: y2, z1: z1, z2: z2}
}

// Contains reports whether (x, y, z) lies inside the box, bounds inclusive.
func (c Cuboid) Contains(x, y, z int) bool {
	return x >= c.x1 && x <= c.x2 && y >= c.y1 && y <= c.y2 && z >= c.z1 && z <= c.z2
}

// IntersectsRect reports whether the box's footprint overlaps the rectangle.
// Corner-in-box probes run one unit below the top z plane, so a box with
// zero z extent reports no corner hits; edge crossings still count.
func (c Cuboid) IntersectsRect(ax1, ax2, ay1, ay2 int) bool {
	// Any rectangle corner inside this box?
	probeZ := c.z2 - 1
	if c.Contains(ax1, ay1, probeZ) || c.Contains(ax1, ay2, probeZ) || c.Contains(ax2, ay1, probeZ) || c.Contains(ax2, ay2, probeZ) {
		return true
	}
	// Any box corner strictly inside the rectangle?
	if c.x1 > ax1 && c.x1 < ax2 && c.y1 > ay1 && c.y1 < ay2 {
		return true
	}
	if c.x1 > ax1 && c.x1 < ax2 && c.y2 > ay1 && c.y2 < ay2 {
		return true
	}
	if c.x2 > ax1 && c.x2 < ax2 && c.y1 > ay1 && c.y1 < ay2 {
		return true
	}
	if c.x2 > ax1 && c.x2 < ax2 && c.y2 > ay1 && c.y2 < ay2 {
		return true
	}
	// Horizontal box edges against vertical rectangle edges.
	if segmentsIntersect(c.x1, c.y1, c.x2, c.y1, ax1, ay1, ax1, ay2) {
		return true
	}
	if segmentsIntersect(c.x1, c.y1, c.x2, c.y1, ax2, ay1, ax2, ay2) {
		return true
	}
	if segmentsIntersect(c.x1, c.y2, c.x2, c.y2, ax1, ay1, ax1, ay2) {
		return true
	}
	if segmentsIntersect(c.x1, c.y2, c.x2, c.y2, ax2, ay1, ax2, ay2) {
		return true
	}
	// Vertical box edges against horizontal rectangle edges.
	if segmentsIntersect(c.x1, c.y1, c.x1, c.y2, ax1, ay1, ax2, ay1) {
		return true
	}
	if segmentsIntersect(c.x1, c.y1, c.x1, c.y2, ax1, ay2, ax2, ay2) {
		return true
	}
	if segmentsIntersect(c.x2, c.y1, c.x2, c.y2, ax1, ay1, ax2, ay1) {
		return true
	}
	if segmentsIntersect(c.x2, c.y1, c.x2, c.y2, ax1, ay2, ax2, ay2) {
		return true
	}
	return false
}

// LowZ is the box's lower z bound.
func (c Cuboid) LowZ() int { return c.z1 }

// HighZ is the box's upper z bound.
func (c Cuboid) HighZ() int { return c.z2 }

// Cylinder is a vertical circular column.
type Cylinder struct {
	x, y, z1, z2, rad int
	radSq             int64
}

// NewCylinder builds a Cylinder centered on (x, y) spanning z1..z2. The
// radius must be positive.
func NewCylinder(x, y, z1, z2, rad int) (Cylinder, error) {
	if rad <= 0 {
		return Cylinder{}, fmt.Errorf("zone: cylinder radius must be positive, got %d", rad)
	}
	return Cylinder{x: x, y: y, z1: z1, z2: z2, rad: rad, radSq: int64(rad) * int64(rad)}, nil
}

// Contains reports whether (x, y, z) lies inside the column: squared
// planar distance at most radius squared, z bounds inclusive. Integer
// squared distances stay well below 2^53, so this matches a
// double-precision evaluation exactly.
func (c Cylinder) Contains(x, y, z int) bool {
	dx, dy := int64(c.x-x), int64(c.y-y)
	return dx*dx+dy*dy <= c.radSq && z >= c.z1 && z <= c.z2
}

// IntersectsRect reports whether the column's footprint overlaps the
// rectangle. Corner and side probes are strict: a circle exactly tangent
// to a rectangle side does not count as overlapping.
func (c Cylinder) IntersectsRect(ax1, ax2, ay1, ay2 int) bool {
	// Center strictly inside the rectangle?
	if c.x > ax1 && c.x < ax2 && c.y > ay1 && c.y < ay2 {
		return true
	}
	// Any rectangle corner strictly inside the circle?
	dist := func(px, py int) int64 {
		dx, dy := int64(px-c.x), int64(py-c.y)
		return dx*dx + dy*dy
	}
	if dist(ax1, ay1) < c.radSq || dist(ax1, ay2) < c.radSq || dist(ax2, ay1) < c.radSq || dist(ax2, ay2) < c.radSq {
		return true
	}
	// Circle crossing a side of the rectangle?
	abs := func(v int) int {
		if v < 0 {
			return -v
		}
		return v
	}
	if c.x > ax1 && c.x < ax2 {
		if abs(c.y-ay2) < c.rad || abs(c.y-ay1) < c.rad {
			return true
		}
	}
	if c.y > ay1 && c.y < ay2 {
		if abs(c.x-ax2) < c.rad || abs(c.x-ax1) < c.rad {
			return true
		}
	}
	return false
}

// LowZ is the column's lower z bound.
func (c Cylinder) LowZ() int { return c.z1 }

// HighZ is the column's upper z bound.
func (c Cylinder) HighZ() int { return c.z2 }

// Polygon is a vertical prism over an arbitrary simple polygon.
type Polygon struct {
	xs, ys []int32
	z1, z2 int
}

// NewPolygon builds a Polygon from at least three vertices spanning z1..z2.
func NewPolygon(nodes []location.Point, z1, z2 int) (Polygon, error) {
	if len(nodes) < 3 {
		return Polygon{}, fmt.Errorf("zone: polygon needs at least 3 vertices, got %d", len(nodes))
	}
	xs := make([]int32, len(nodes))
	ys := make([]int32, len(nodes))
	for i, n := range nodes {
		xs[i] = int32(n.X)
		ys[i] = int32(n.Y)
	}
	return Polygon{xs: xs, ys: ys, z1: z1, z2: z2}, nil
}

// Contains reports whether (x, y, z) lies inside the prism, using an
// edge-crossing ray cast. The crossing test deliberately runs in 32-bit
// integer arithmetic with truncating division - the parity of edge cases
// on vertices and slanted edges depends on it.
func (p Polygon) Contains(x, y, z int) bool {
	if z < p.z1 || z > p.z2 {
		return false
	}
	px, py := int32(x), int32(y)
	inside := false
	for i, j := 0, len(p.xs)-1; i < len(p.xs); j, i = i, i+1 {
		yi, yj := p.ys[i], p.ys[j]
		if ((yi <= py && py < yj) || (yj <= py && py < yi)) &&
			px < (p.xs[j]-p.xs[i])*(py-yi)/(yj-yi)+p.xs[i] {
			inside = !inside
		}
	}
	return inside
}

// IntersectsRect reports whether the prism's footprint overlaps the
// rectangle: first vertex strictly inside the rectangle, rectangle corner
// inside the polygon (probed one unit below the top z plane), or any
// polygon edge crossing any rectangle side.
func (p Polygon) IntersectsRect(ax1, ax2, ay1, ay2 int) bool {
	if int(p.xs[0]) > ax1 && int(p.xs[0]) < ax2 && int(p.ys[0]) > ay1 && int(p.ys[0]) < ay2 {
		return true
	}
	if p.Contains(ax1, ay1, p.z2-1) {
		return true
	}
	for i := range p.xs {
		tx, ty := int(p.xs[i]), int(p.ys[i])
		next := (i + 1) % len(p.xs)
		ux, uy := int(p.xs[next]), int(p.ys[next])
		if segmentsIntersect(tx, ty, ux, uy, ax1, ay1, ax1, ay2) {
			return true
		}
		if segmentsIntersect(tx, ty, ux, uy, ax1, ay1, ax2, ay1) {
			return true
		}
		if segmentsIntersect(tx, ty, ux, uy, ax2, ay2, ax1, ay2) {
			return true
		}
		if segmentsIntersect(tx, ty, ux, uy, ax2, ay2, ax2, ay1) {
			return true
		}
	}
	return false
}

// LowZ is the prism's lower z bound.
func (p Polygon) LowZ() int { return p.z1 }

// HighZ is the prism's upper z bound.
func (p Polygon) HighZ() int { return p.z2 }
