package geometry

import "fmt"

// triangulationMaxLoops is the ear-search budget: after this many passes
// without reducing the ring to a triangle, the input is rejected as
// non-monotone.
const triangulationMaxLoops = 100

// Triangulate ear-clips points into the triangle list that describes the
// same polygon. points must form a simple, monotone polygon; anything else
// is rejected after triangulationMaxLoops ear-search passes.
//
// Every intermediate product is computed in int32 so that oversized world
// coordinates wrap exactly where the reference's 32-bit integer arithmetic
// wraps: the orientation and convexity tests multiply an absolute
// coordinate by an edge delta, which can exceed int32 for a wide polygon,
// and the resulting sign is what decides the clipping order.
func Triangulate(points []Point) ([]Triangle, error) {
	if len(points) < 3 {
		return nil, fmt.Errorf("geometry: polygon needs at least 3 vertices, got %d", len(points))
	}
	ring, err := newVertexList(points)
	if err != nil {
		return nil, err
	}
	return triangulate(ring)
}

// triangulate is Triangulate over an already-validated vertex ring.
func triangulate(ring vertexList) ([]Triangle, error) {
	xs := append([]int32(nil), ring.xs...)
	ys := append([]int32(nil), ring.ys...)
	isCw := polygonOrientation(xs, ys)
	ncx, ncy := nonConvexPoints(xs, ys, isCw)

	var triangles []Triangle
	size := len(xs)
	index := 1
	loops := 0
	for size > 3 {
		indexPrev := prevIndex(size, index)
		indexNext := nextIndex(size, index)

		if isEar(isCw, ncx, ncy,
			xs[indexPrev], ys[indexPrev],
			xs[index], ys[index],
			xs[indexNext], ys[indexNext]) {
			triangles = append(triangles, newTriangleFromCoords(
				xs[indexPrev], ys[indexPrev],
				xs[index], ys[index],
				xs[indexNext], ys[indexNext]))
			xs = append(xs[:index], xs[index+1:]...)
			ys = append(ys[:index], ys[index+1:]...)
			size--
			index = prevIndex(size, index)
		} else {
			index = indexNext
		}

		loops++
		if loops == triangulationMaxLoops {
			return nil, fmt.Errorf("geometry: vertices do not form a monotone polygon")
		}
	}
	return append(triangles, newTriangleFromCoords(xs[0], ys[0], xs[1], ys[1], xs[2], ys[2])), nil
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

// polygonOrientation reports whether the ring is wound clockwise, deciding
// it at the vertex with the smallest x (ties broken by the largest y).
func polygonOrientation(xs, ys []int32) bool {
	index := 0
	px, py := xs[0], ys[0]
	for i := 1; i < len(xs); i++ {
		if xs[i] < px || (xs[i] == px && ys[i] > py) {
			px, py = xs[i], ys[i]
			index = i
		}
	}
	size := len(xs)
	prev := prevIndex(size, index)
	next := nextIndex(size, index)
	vx := px - xs[prev]
	vy := py - ys[prev]
	return xs[next]*vy-ys[next]*vx+vx*ys[prev]-vy*xs[prev] <= 0
}

// nonConvexPoints collects the ring's reflex vertices — the only ones an
// ear candidate has to be tested against.
func nonConvexPoints(xs, ys []int32, isCw bool) (ncx, ncy []int32) {
	size := len(xs)
	for i := 0; i < size-1; i++ {
		next := nextIndex(size, i)
		nextNext := nextIndex(size, i+1)
		vx := xs[next] - xs[i]
		vy := ys[next] - ys[i]
		if (xs[nextNext]*vy-ys[nextNext]*vx+vx*ys[i]-vy*xs[i] > 0) == isCw {
			ncx = append(ncx, xs[next])
			ncy = append(ncy, ys[next])
		}
	}
	return ncx, ncy
}

// isConvex reports whether vertex B of the consecutive triple A, B, C turns
// the same way as the ring as a whole.
func isConvex(isCw bool, ax, ay, bx, by, cx, cy int32) bool {
	bax := bx - ax
	bay := by - ay
	return (cx*bay-cy*bax+bax*ay-bay*ax > 0) != isCw
}

// earPointInTriangle is the ear-detection containment test: strictly
// interior, computed as float-division barycentric coefficients. It is
// deliberately distinct from Triangle.Contains, which is edge-inclusive and
// runs in exact integers.
func earPointInTriangle(ax, ay, bx, by, cx, cy, px, py int32) bool {
	bax := float64(bx - ax)
	bay := float64(by - ay)
	cax := float64(cx - ax)
	cay := float64(cy - ay)
	pax := float64(px - ax)
	pay := float64(py - ay)

	det := bax*cay - cax*bay
	ba := (bax*pay - pax*bay) / det
	ca := (pax*cay - cax*pay) / det
	return ba > 0 && ca > 0 && (ba+ca) < 1
}

// isEar reports whether triangle ABC can be clipped off: B is convex and no
// reflex vertex of the ring falls inside ABC.
func isEar(isCw bool, ncx, ncy []int32, ax, ay, bx, by, cx, cy int32) bool {
	if !isConvex(isCw, ax, ay, bx, by, cx, cy) {
		return false
	}
	for i := range ncx {
		if earPointInTriangle(ax, ay, bx, by, cx, cy, ncx[i], ncy[i]) {
			return false
		}
	}
	return true
}
