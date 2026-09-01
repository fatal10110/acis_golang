package geometry

import (
	"math"
	"testing"
)

// ---- from circle_test.go ----
func TestNewCircleRejectsNonPositiveRadius(t *testing.T) {
	for _, rad := range []int{0, -5} {
		if _, err := NewCircle(0, 0, rad); err == nil {
			t.Errorf("NewCircle(rad=%d) succeeded, want error", rad)
		}
	}
}

func TestCircleContains(t *testing.T) {
	c, err := NewCircle(0, 0, 10)
	if err != nil {
		t.Fatalf("NewCircle: %v", err)
	}
	cases := []struct {
		x, y int
		want bool
	}{
		{0, 0, true},
		{10, 0, true}, // on the boundary, inclusive
		{8, 8, false}, // distance sqrt(128) > 10
		{11, 0, false},
	}
	for _, c2 := range cases {
		if got := c.Contains(c2.x, c2.y); got != c2.want {
			t.Errorf("Contains(%d, %d) = %v, want %v", c2.x, c2.y, got, c2.want)
		}
	}
}

func TestCircleArea(t *testing.T) {
	c, err := NewCircle(0, 0, 10)
	if err != nil {
		t.Fatalf("NewCircle: %v", err)
	}
	want := math.Pi * 100
	if got := c.Area(); math.Abs(got-want) > 1e-9 {
		t.Errorf("Area() = %v, want %v", got, want)
	}
}

func TestCircleIntersectsRect(t *testing.T) {
	c, err := NewCircle(0, 0, 10)
	if err != nil {
		t.Fatalf("NewCircle: %v", err)
	}
	cases := []struct {
		name           string
		x1, x2, y1, y2 int
		want           bool
	}{
		{"center inside rect", -5, 5, -5, 5, true},
		{"disjoint", 100, 200, 100, 200, false},
		{"rect corner inside circle", 5, 20, 5, 20, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.IntersectsRect(tc.x1, tc.x2, tc.y1, tc.y2); got != tc.want {
				t.Errorf("IntersectsRect(%d,%d,%d,%d) = %v, want %v", tc.x1, tc.x2, tc.y1, tc.y2, got, tc.want)
			}
		})
	}
}

// ---- from intersect_test.go ----
func TestIntersectsCrossKind(t *testing.T) {
	rect := NewRectangle(0, 10, 0, 10)
	circleOverlap, err := NewCircle(5, 5, 3)
	if err != nil {
		t.Fatalf("NewCircle: %v", err)
	}
	circleDisjoint, err := NewCircle(100, 100, 3)
	if err != nil {
		t.Fatalf("NewCircle: %v", err)
	}
	tri, err := NewTriangle(Point{20, 0}, Point{30, 0}, Point{20, 10})
	if err != nil {
		t.Fatalf("NewTriangle: %v", err)
	}
	poly, err := NewPolygon([]Point{{8, 8}, {15, 8}, {15, 15}, {8, 15}})
	if err != nil {
		t.Fatalf("NewPolygon: %v", err)
	}
	nested := NewRectangle(2, 8, 2, 8)

	cases := []struct {
		name string
		a, b Shape
		want bool
	}{
		{"rect vs circle overlapping", rect, circleOverlap, true},
		{"circle vs rect overlapping (symmetric)", circleOverlap, rect, true},
		{"rect vs circle disjoint", rect, circleDisjoint, false},
		{"rect vs triangle disjoint", rect, tri, false},
		{"rect vs polygon overlapping edge", rect, poly, true},
		{"rect vs nested rect (fully contains)", rect, nested, true},
		{"circle vs circle overlapping", circleOverlap, circleOverlap, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.a.Intersects(c.b); got != c.want {
				t.Errorf("Intersects() = %v, want %v", got, c.want)
			}
		})
	}
}

// ---- from polygon_test.go ----
func TestNewPolygonRejectsTooFewVertices(t *testing.T) {
	if _, err := NewPolygon([]Point{{0, 0}, {1, 1}}); err == nil {
		t.Error("NewPolygon with 2 vertices succeeded, want error")
	}
}

func TestPolygonContainsConcave(t *testing.T) {
	// An L-shaped (concave) polygon: a 10x10 square with a 5x5 notch bitten
	// out of its top-right corner.
	poly, err := NewPolygon([]Point{
		{0, 0}, {10, 0}, {10, 5}, {5, 5}, {5, 10}, {0, 10},
	})
	if err != nil {
		t.Fatalf("NewPolygon: %v", err)
	}
	cases := []struct {
		x, y int
		want bool
	}{
		{2, 2, true},    // interior, main body
		{8, 8, false},   // inside the notch, not the polygon
		{2, 8, true},    // interior, left arm of the L
		{20, 20, false}, // clearly outside
	}
	for _, c := range cases {
		if got := poly.Contains(c.x, c.y); got != c.want {
			t.Errorf("Contains(%d, %d) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
}

func TestPolygonArea(t *testing.T) {
	poly, err := NewPolygon([]Point{{0, 0}, {10, 0}, {10, 10}, {0, 10}})
	if err != nil {
		t.Fatalf("NewPolygon: %v", err)
	}
	if got := poly.Area(); got != 100 {
		t.Errorf("Area() = %v, want 100", got)
	}
}

// ---- from rectangle_test.go ----
func TestRectangleContains(t *testing.T) {
	r := NewRectangle(0, 10, 0, 10)
	cases := []struct {
		x, y int
		want bool
	}{
		{0, 0, true},   // corner, inclusive
		{10, 10, true}, // opposite corner, inclusive
		{5, 5, true},   // interior
		{-1, 5, false},
		{5, -1, false},
		{11, 5, false},
		{5, 11, false},
	}
	for _, c := range cases {
		if got := r.Contains(c.x, c.y); got != c.want {
			t.Errorf("Contains(%d, %d) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
}

func TestRectangleNormalizesCorners(t *testing.T) {
	a := NewRectangle(0, 10, 0, 20)
	b := NewRectangle(10, 0, 20, 0)
	if a.x1 != b.x1 || a.x2 != b.x2 || a.y1 != b.y1 || a.y2 != b.y2 {
		t.Errorf("reversed corners produced different bounds: %+v vs %+v", a, b)
	}
}

func TestRectangleArea(t *testing.T) {
	r := NewRectangle(0, 10, 0, 20)
	if got := r.Area(); got != 200 {
		t.Errorf("Area() = %v, want 200", got)
	}
}

func TestRectangleIntersectsRect(t *testing.T) {
	r := NewRectangle(0, 10, 0, 10)
	cases := []struct {
		name           string
		x1, x2, y1, y2 int
		want           bool
	}{
		{"overlapping", 5, 15, 5, 15, true},
		{"disjoint", 20, 30, 20, 30, false},
		{"fully inside", 2, 8, 2, 8, true},
		{"fully contains", -5, 15, -5, 15, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := r.IntersectsRect(c.x1, c.x2, c.y1, c.y2); got != c.want {
				t.Errorf("IntersectsRect(%d,%d,%d,%d) = %v, want %v", c.x1, c.x2, c.y1, c.y2, got, c.want)
			}
		})
	}
}

// ---- from territory_test.go ----
func TestNewTerritoryRejectsNoShapes(t *testing.T) {
	if _, err := NewTerritory(0, 10); err == nil {
		t.Error("NewTerritory with no shapes succeeded, want error")
	}
}

func TestNewTerritoryAcceptsInvertedZRangeAsAlwaysEmpty(t *testing.T) {
	rect := NewRectangle(0, 10, 0, 10)
	tr, err := NewTerritory(10, 0, rect)
	if err != nil {
		t.Fatalf("NewTerritory with minZ > maxZ: %v", err)
	}
	if tr.Contains(5, 5, 5) {
		t.Error("Contains matched a z inside an inverted range, want never true")
	}
}

func TestTerritoryContainsUnionAndZRange(t *testing.T) {
	a := NewRectangle(0, 10, 0, 10)
	b := NewRectangle(100, 110, 100, 110)
	tr, err := NewTerritory(-50, 50, a, b)
	if err != nil {
		t.Fatalf("NewTerritory: %v", err)
	}
	cases := []struct {
		x, y, z int
		want    bool
	}{
		{5, 5, 0, true},     // inside shape a, mid z-range
		{105, 105, 0, true}, // inside shape b (union)
		{5, 5, -50, true},   // z at the low bound, inclusive
		{5, 5, 50, true},    // z at the high bound, inclusive
		{5, 5, -51, false},  // z just below the range
		{5, 5, 51, false},   // z just above the range
		{50, 50, 0, false},  // between the two shapes, in neither
	}
	for _, c := range cases {
		if got := tr.Contains(c.x, c.y, c.z); got != c.want {
			t.Errorf("Contains(%d,%d,%d) = %v, want %v", c.x, c.y, c.z, got, c.want)
		}
	}
}

func TestTerritoryLowHighZ(t *testing.T) {
	tr, err := NewTerritory(-10, 20, NewRectangle(0, 1, 0, 1))
	if err != nil {
		t.Fatalf("NewTerritory: %v", err)
	}
	if tr.LowZ() != -10 || tr.HighZ() != 20 {
		t.Errorf("LowZ/HighZ = %d/%d, want -10/20", tr.LowZ(), tr.HighZ())
	}
}

func TestTerritoryArea(t *testing.T) {
	a := NewRectangle(0, 10, 0, 10) // area 100
	b := NewRectangle(0, 5, 0, 5)   // area 25, overlaps a but is not deduplicated
	tr, err := NewTerritory(0, 10, a, b)
	if err != nil {
		t.Fatalf("NewTerritory: %v", err)
	}
	if got := tr.Area(); got != 125 {
		t.Errorf("Area() = %v, want 125 (sum, not union)", got)
	}
}

func TestTerritoryIntersects(t *testing.T) {
	overlapping, err := NewTerritory(0, 10, NewRectangle(0, 10, 0, 10))
	if err != nil {
		t.Fatalf("NewTerritory: %v", err)
	}
	sameXYDisjointZ, err := NewTerritory(20, 30, NewRectangle(0, 10, 0, 10))
	if err != nil {
		t.Fatalf("NewTerritory: %v", err)
	}
	sameZDisjointXY, err := NewTerritory(0, 10, NewRectangle(1000, 1010, 1000, 1010))
	if err != nil {
		t.Fatalf("NewTerritory: %v", err)
	}
	both, err := NewTerritory(5, 15, NewRectangle(5, 15, 5, 15))
	if err != nil {
		t.Fatalf("NewTerritory: %v", err)
	}

	if overlapping.Intersects(sameXYDisjointZ) {
		t.Error("territories with disjoint z ranges reported as intersecting")
	}
	if overlapping.Intersects(sameZDisjointXY) {
		t.Error("territories with disjoint footprints reported as intersecting")
	}
	if !overlapping.Intersects(both) {
		t.Error("territories overlapping in both z range and footprint reported as not intersecting")
	}
}

// ---- from triangle_test.go ----
func TestTriangleContains(t *testing.T) {
	tri, err := NewTriangle(Point{0, 0}, Point{10, 0}, Point{0, 10})
	if err != nil {
		t.Fatalf("NewTriangle: %v", err)
	}
	cases := []struct {
		x, y int
		want bool
	}{
		{1, 1, true},  // interior, near the right-angle corner
		{0, 0, true},  // vertex
		{9, 9, false}, // outside the hypotenuse
		{-1, 1, false},
	}
	for _, c := range cases {
		if got := tri.Contains(c.x, c.y); got != c.want {
			t.Errorf("Contains(%d, %d) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
}

func TestTriangleCenter(t *testing.T) {
	tri, err := NewTriangle(Point{1, 1}, Point{4, 1}, Point{1, 7})
	if err != nil {
		t.Fatalf("NewTriangle: %v", err)
	}
	// Integer centroid: (1+4+1)/3 = 2, (1+1+7)/3 = 3.
	if got := tri.Center(); got != (Point{X: 2, Y: 3}) {
		t.Errorf("Center() = %+v, want {2 3}", got)
	}
}

func TestTriangleArea(t *testing.T) {
	tri, err := NewTriangle(Point{0, 0}, Point{10, 0}, Point{0, 10})
	if err != nil {
		t.Fatalf("NewTriangle: %v", err)
	}
	if got := tri.Area(); got != 50 {
		t.Errorf("Area() = %v, want 50", got)
	}
}
