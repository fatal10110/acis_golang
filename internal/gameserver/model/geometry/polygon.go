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

// Polygon is a simple (non-self-intersecting) 2D polygon of arbitrary
// vertex count, convex or concave.
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
