// Package geometry provides 2D shape primitives and a 3D Territory composite
// built from them. It has no knowledge of zones, spawns, or any other
// domain — it is pure shape math shared by every package that needs to
// describe a region of the world.
package geometry

import (
	"fmt"
	"math"
)

// Shape is a 2D footprint that can answer point containment, area, and
// overlap against another Shape or an axis-aligned rectangle.
type Shape interface {
	// Contains reports whether (x, y) lies inside the shape, bounds
	// inclusive.
	Contains(x, y int) bool
	// Area is the shape's 2D area.
	Area() float64
	// IntersectsRect reports whether the shape's footprint overlaps the
	// axis-aligned rectangle spanning x1..x2 by y1..y2.
	IntersectsRect(x1, x2, y1, y2 int) bool
	// Intersects reports whether the shape overlaps other.
	Intersects(other Shape) bool
}

// Point is a 2D vertex.
type Point struct {
	X, Y int
}

// EdgesCross reports whether any edge of the closed vertex ring a crosses
// any edge of the closed vertex ring b (shared endpoints and collinear
// overlap both count). It does not check whether one ring's vertices lie
// inside the other — only whether their boundaries touch.
func EdgesCross(a, b []Point) bool {
	for i := range a {
		j := (i + 1) % len(a)
		for k := range b {
			l := (k + 1) % len(b)
			if segmentsIntersect(a[i].X, a[i].Y, a[j].X, a[j].Y, b[k].X, b[k].Y, b[l].X, b[l].Y) {
				return true
			}
		}
	}
	return false
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

// vertexRingContains reports whether (x, y) lies inside the closed vertex
// ring (xs, ys) using an edge-crossing ray cast. The crossing test
// deliberately runs in 32-bit integer arithmetic with truncating division —
// the parity of edge cases on vertices and slanted edges depends on it.
func vertexRingContains(xs, ys []int32, x, y int) bool {
	px, py := int32(x), int32(y)
	inside := false
	for i, j := 0, len(xs)-1; i < len(xs); j, i = i, i+1 {
		yi, yj := ys[i], ys[j]
		if ((yi <= py && py < yj) || (yj <= py && py < yi)) &&
			px < (xs[j]-xs[i])*(py-yi)/(yj-yi)+xs[i] {
			inside = !inside
		}
	}
	return inside
}

// vertexRingArea computes a closed vertex ring's area via the shoelace
// formula.
func vertexRingArea(xs, ys []int32) float64 {
	var sum int64
	for i, j := 0, len(xs)-1; i < len(xs); j, i = i, i+1 {
		sum += int64(xs[j])*int64(ys[i]) - int64(xs[i])*int64(ys[j])
	}
	if sum < 0 {
		sum = -sum
	}
	return float64(sum) / 2
}

// vertexRingIntersectsRect reports whether a closed vertex ring's footprint
// overlaps the rectangle: first vertex strictly inside the rectangle,
// rectangle corner inside the ring, or any ring edge crossing any rectangle
// side.
func vertexRingIntersectsRect(xs, ys []int32, ax1, ax2, ay1, ay2 int) bool {
	if int(xs[0]) > ax1 && int(xs[0]) < ax2 && int(ys[0]) > ay1 && int(ys[0]) < ay2 {
		return true
	}
	if vertexRingContains(xs, ys, ax1, ay1) {
		return true
	}
	for i := range xs {
		tx, ty := int(xs[i]), int(ys[i])
		next := (i + 1) % len(xs)
		ux, uy := int(xs[next]), int(ys[next])
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

// vertexRingsIntersect reports whether two closed, simple vertex rings
// overlap: any edge pair crosses, or either ring's first vertex lies inside
// the other. Works for convex and concave rings alike, unlike a plain
// separating-axis test.
func vertexRingsIntersect(axs, ays, bxs, bys []int32) bool {
	for i := range axs {
		j := (i + 1) % len(axs)
		for k := range bxs {
			l := (k + 1) % len(bxs)
			if segmentsIntersect(int(axs[i]), int(ays[i]), int(axs[j]), int(ays[j]),
				int(bxs[k]), int(bys[k]), int(bxs[l]), int(bys[l])) {
				return true
			}
		}
	}
	return vertexRingContains(bxs, bys, int(axs[0]), int(ays[0])) ||
		vertexRingContains(axs, ays, int(bxs[0]), int(bys[0]))
}

// int32Vertex narrows v to int32, failing if it overflows. World
// coordinates always fit in 32 bits, so an out-of-range value marks
// malformed input rather than a legitimate vertex.
func int32Vertex(v int) (int32, error) {
	if v < math.MinInt32 || v > math.MaxInt32 {
		return 0, fmt.Errorf("value %d overflows int32", v)
	}
	return int32(v), nil
}

// pointSegmentDistSq returns the squared distance from (px, py) to the
// closest point on segment (x1,y1)-(x2,y2).
func pointSegmentDistSq(px, py, x1, y1, x2, y2 int) float64 {
	fx1, fy1, fx2, fy2 := float64(x1), float64(y1), float64(x2), float64(y2)
	fpx, fpy := float64(px), float64(py)
	dx, dy := fx2-fx1, fy2-fy1
	lenSq := dx*dx + dy*dy
	if lenSq == 0 {
		ddx, ddy := fpx-fx1, fpy-fy1
		return ddx*ddx + ddy*ddy
	}
	t := ((fpx-fx1)*dx + (fpy-fy1)*dy) / lenSq
	switch {
	case t < 0:
		t = 0
	case t > 1:
		t = 1
	}
	cx, cy := fx1+t*dx, fy1+t*dy
	ddx, ddy := fpx-cx, fpy-cy
	return ddx*ddx + ddy*ddy
}
