package geometry

// Rectangle is an axis-aligned 2D box.
type Rectangle struct {
	vertexList
	x1, x2, y1, y2 int
}

// NewRectangle builds a Rectangle from two opposite corners; coordinates
// may be given in either order on each axis.
func NewRectangle(x1, x2, y1, y2 int) Rectangle {
	if x1 > x2 {
		x1, x2 = x2, x1
	}
	if y1 > y2 {
		y1, y2 = y2, y1
	}
	// Corners in ring order, so the shared vertex-list math (Area,
	// IntersectsRect, Intersects) applies unchanged.
	vl, _ := newVertexList([]Point{{x1, y1}, {x2, y1}, {x2, y2}, {x1, y2}})
	return Rectangle{vertexList: vl, x1: x1, x2: x2, y1: y1, y2: y2}
}

// Contains reports whether (x, y) lies inside the box, bounds inclusive.
// This overrides the embedded vertex-list ray cast, which does not treat
// every edge as inside.
func (r Rectangle) Contains(x, y int) bool {
	return x >= r.x1 && x <= r.x2 && y >= r.y1 && y <= r.y2
}
