package geometry

// polygonal is satisfied by every vertex-ring-backed shape (Rectangle,
// Triangle, Polygon), letting intersects route them through one generic
// ring-vs-ring test instead of a hand-written case per pair of concrete
// types.
type polygonal interface {
	vertices() (xs, ys []int32)
}

// intersects is the single dispatch point every Shape.Intersects
// implementation calls through. Circle pairs get closed-form tests;
// vertex-ring pairs (Rectangle, Triangle, Polygon, in any combination) share
// one ring-vs-ring test.
func intersects(a, b Shape) bool {
	ac, aCircle := a.(Circle)
	bc, bCircle := b.(Circle)
	switch {
	case aCircle && bCircle:
		return circlesIntersect(ac, bc)
	case aCircle:
		return circlePolygonIntersects(ac, b.(polygonal))
	case bCircle:
		return circlePolygonIntersects(bc, a.(polygonal))
	default:
		axs, ays := a.(polygonal).vertices()
		bxs, bys := b.(polygonal).vertices()
		return vertexRingsIntersect(axs, ays, bxs, bys)
	}
}

// circlesIntersect reports whether two circles overlap: center distance
// strictly less than the radius sum, so externally tangent circles do not
// count as overlapping.
func circlesIntersect(a, b Circle) bool {
	dx, dy := int64(a.x-b.x), int64(a.y-b.y)
	distSq := dx*dx + dy*dy
	radSum := int64(a.rad + b.rad)
	return distSq < radSum*radSum
}

// circlePolygonIntersects reports whether a circle overlaps a vertex-ring
// shape: the circle's center inside the ring, any ring vertex inside the
// circle, or any ring edge passing within the radius of the center.
func circlePolygonIntersects(c Circle, p polygonal) bool {
	xs, ys := p.vertices()
	if vertexRingContains(xs, ys, c.x, c.y) {
		return true
	}
	for i, j := 0, len(xs)-1; i < len(xs); j, i = i, i+1 {
		if pointSegmentDistSq(c.x, c.y, int(xs[j]), int(ys[j]), int(xs[i]), int(ys[i])) < float64(c.radSq) {
			return true
		}
	}
	return false
}
