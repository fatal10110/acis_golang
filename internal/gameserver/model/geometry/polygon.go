package geometry

import "fmt"

// vertexList is the vertex-ring representation shared by Triangle and
// Polygon: identical containment, area, and overlap math regardless of
// vertex count.
type vertexList struct {
	xs, ys []int32
}

func newVertexList(points []Point) (vertexList, error) {
	xs := make([]int32, len(points))
	ys := make([]int32, len(points))
	for i, p := range points {
		x, err := int32Vertex(p.X)
		if err != nil {
			return vertexList{}, fmt.Errorf("geometry: vertex %d: x: %w", i, err)
		}
		y, err := int32Vertex(p.Y)
		if err != nil {
			return vertexList{}, fmt.Errorf("geometry: vertex %d: y: %w", i, err)
		}
		xs[i], ys[i] = x, y
	}
	return vertexList{xs: xs, ys: ys}, nil
}

// Contains reports whether (x, y) lies inside the vertex ring.
func (v vertexList) Contains(x, y int) bool { return vertexRingContains(v.xs, v.ys, x, y) }

// Area is the vertex ring's area.
func (v vertexList) Area() float64 { return vertexRingArea(v.xs, v.ys) }

// IntersectsRect reports whether the vertex ring's footprint overlaps the
// rectangle.
func (v vertexList) IntersectsRect(x1, x2, y1, y2 int) bool {
	return vertexRingIntersectsRect(v.xs, v.ys, x1, x2, y1, y2)
}

// Intersects reports whether v overlaps other, dispatching on other's kind.
func (v vertexList) Intersects(other Shape) bool { return intersects(v, other) }

func (v vertexList) vertices() (xs, ys []int32) { return v.xs, v.ys }

// Vertices returns the shape's vertex ring in traversal order (the ring is
// implicitly closed: the last vertex connects back to the first).
func (v vertexList) Vertices() []Point {
	out := make([]Point, len(v.xs))
	for i := range v.xs {
		out[i] = Point{X: int(v.xs[i]), Y: int(v.ys[i])}
	}
	return out
}

// Polygon is a 2D polygon of arbitrary vertex count whose containment is
// even-odd ray casting straight over the vertex ring.
//
// That is the zone-polygon rule, and it is not interchangeable with
// TriangulatedPolygon: ray casting is half-open (a point on the boundary
// counts as inside on one side only) and treats the enclosed-an-odd-number-
// of-times region as inside, so on a concave or self-touching ring the two
// disagree on interior points, not merely on edges. Use this type only
// where the reference defines the region by its raw ring; use
// TriangulatedPolygon everywhere the reference triangulates first.
type Polygon struct {
	vertexList
}

// NewPolygon builds a Polygon from at least three vertices.
func NewPolygon(points []Point) (Polygon, error) {
	if len(points) < 3 {
		return Polygon{}, fmt.Errorf("geometry: polygon needs at least 3 vertices, got %d", len(points))
	}
	vl, err := newVertexList(points)
	if err != nil {
		return Polygon{}, err
	}
	return Polygon{vertexList: vl}, nil
}

// TriangulatedPolygon is a 2D polygon stored as the triangles ear clipping
// produced from its vertex ring. Containment is the union of its triangles,
// each tested edge-inclusively, which is the rule for every region the
// reference builds by triangulating first — spawn territories and door
// footprints. It classifies concave and self-touching rings correctly,
// where ray casting over the same ring does not.
//
// The ring is retained alongside the triangles so area, rectangle overlap,
// and shape-vs-shape intersection keep working; only Contains and Area
// come from the triangles.
type TriangulatedPolygon struct {
	vertexList

	triangles []Triangle
}

// NewTriangulatedPolygon ear-clips at least three vertices into a
// TriangulatedPolygon. It fails when the vertices do not form a monotone
// polygon, the same input the reference rejects.
func NewTriangulatedPolygon(points []Point) (TriangulatedPolygon, error) {
	if len(points) < 3 {
		return TriangulatedPolygon{}, fmt.Errorf("geometry: polygon needs at least 3 vertices, got %d", len(points))
	}
	vl, err := newVertexList(points)
	if err != nil {
		return TriangulatedPolygon{}, err
	}
	triangles, err := triangulate(vl)
	if err != nil {
		return TriangulatedPolygon{}, err
	}
	return TriangulatedPolygon{vertexList: vl, triangles: triangles}, nil
}

// Contains reports whether (x, y) falls inside any of the polygon's
// triangles.
func (p TriangulatedPolygon) Contains(x, y int) bool {
	_, ok := p.ContainingTriangle(x, y)
	return ok
}

// ContainingTriangle returns the first triangle whose inclusive half-plane
// test contains (x, y).
func (p TriangulatedPolygon) ContainingTriangle(x, y int) (Triangle, bool) {
	for _, t := range p.triangles {
		if t.Contains(x, y) {
			return t, true
		}
	}
	return Triangle{}, false
}

// Size is the sum of the polygon's triangle sizes — the weight the
// reference uses when picking a random point, and the reason a
// self-touching ring is not measured by a shoelace sum that would cancel
// its lobes against each other.
func (p TriangulatedPolygon) Size() int64 {
	var sum int64
	for _, t := range p.triangles {
		sum += t.Size()
	}
	return sum
}

// Area is the polygon's 2D area, which is its Size.
func (p TriangulatedPolygon) Area() float64 { return float64(p.Size()) }
