package geometry

// Triangle is a 2D triangle. It keeps the vertex ring for area, rectangle
// overlap, and shape-vs-shape intersection, but answers Contains with the
// reference's exact-integer half-plane test rather than the even-odd ray
// cast the other ring shapes use: a point is inside when it sits on the
// same side of all three edges, edges inclusive.
type Triangle struct {
	vertexList

	// Vertex A plus the BA and CA edge vectors, the form the half-plane
	// test consumes. int32 throughout so the products wrap where the
	// reference's 32-bit integer arithmetic wraps.
	ax, ay   int32
	bax, bay int32
	cax, cay int32
}

// NewTriangle builds a Triangle from its three vertices.
func NewTriangle(a, b, c Point) (Triangle, error) {
	vl, err := newVertexList([]Point{a, b, c})
	if err != nil {
		return Triangle{}, err
	}
	return newTriangleFromCoords(vl.xs[0], vl.ys[0], vl.xs[1], vl.ys[1], vl.xs[2], vl.ys[2]), nil
}

// newTriangleFromCoords builds a Triangle from vertices already narrowed to
// int32, so it cannot fail.
func newTriangleFromCoords(ax, ay, bx, by, cx, cy int32) Triangle {
	return Triangle{
		vertexList: vertexList{xs: []int32{ax, bx, cx}, ys: []int32{ay, by, cy}},
		ax:         ax,
		ay:         ay,
		bax:        bx - ax,
		bay:        by - ay,
		cax:        cx - ax,
		cay:        cy - ay,
	}
}

// Contains reports whether (x, y) lies inside the triangle, edges
// inclusive. The three cross products run in int64 — the reference widens
// them to long for exactly this reason, since world coordinates squared
// overflow 32 bits.
func (t Triangle) Contains(x, y int) bool {
	dx := int64(int32(x) - t.ax)
	dy := int64(int32(y) - t.ay)
	bax, bay := int64(t.bax), int64(t.bay)
	cax, cay := int64(t.cax), int64(t.cay)

	a := (0-dx)*(bay-0)-(bax-0)*(0-dy) >= 0
	b := (bax-dx)*(cay-bay)-(cax-bax)*(bay-dy) >= 0
	c := (cax-dx)*(0-cay)-(0-cax)*(cay-dy) >= 0
	return a == b && b == c
}

// Size is the triangle's floor-projection size: half the absolute cross
// product of its edge vectors, truncated to a whole number. It is what
// weights a triangle when a polygon picks one at random, so the truncation
// is part of the contract.
func (t Triangle) Size() int64 {
	cross := int64(t.bax*t.cay - t.cax*t.bay)
	if cross < 0 {
		cross = -cross
	}
	return cross / 2
}

// Area is the triangle's 2D area, which is its Size.
func (t Triangle) Area() float64 { return float64(t.Size()) }

// Center is the integer centroid of the three vertices: (Ax+Bx+Cx)/3,
// (Ay+By+Cy)/3, truncating toward zero.
func (t Triangle) Center() Point {
	return Point{
		X: (int(t.ax) + int(t.ax+t.bax) + int(t.ax+t.cax)) / 3,
		Y: (int(t.ay) + int(t.ay+t.bay) + int(t.ay+t.cay)) / 3,
	}
}
