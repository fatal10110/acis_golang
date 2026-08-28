package geometry

import (
	"math"
	"testing"
)

// TestTriangulatedPolygonContainsSelfTouchingVertex locks the triangulated
// containment rule for a self-touching ring, where even-odd ray casting over
// the raw vertex ring disagrees at the shared vertex.
func TestTriangulatedPolygonContainsSelfTouchingVertex(t *testing.T) {
	points := []Point{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 5, Y: 5},
		{X: 10, Y: 10}, {X: 0, Y: 10}, {X: 5, Y: 5},
	}
	poly, err := NewTriangulatedPolygon(points)
	if err != nil {
		t.Fatalf("NewTriangulatedPolygon() error = %v", err)
	}
	if !poly.Contains(5, 5) {
		t.Fatal("Contains(5, 5) = false, want true (triangulation includes the shared vertex)")
	}

	ring, err := NewPolygon(points)
	if err != nil {
		t.Fatalf("NewPolygon() error = %v", err)
	}
	if ring.Contains(5, 5) {
		t.Fatal("Polygon.Contains(5, 5) = true; the two algorithms are expected to disagree here, " +
			"so this test no longer proves the triangulated path is the one in use")
	}
}

func TestTriangulatedPolygonContainsConcaveNotch(t *testing.T) {
	// L-shaped concave polygon; (7,7) falls in the notch cut out of the
	// bounding box and must be classified outside.
	poly, err := NewTriangulatedPolygon([]Point{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 4},
		{X: 4, Y: 4}, {X: 4, Y: 10}, {X: 0, Y: 10},
	})
	if err != nil {
		t.Fatalf("NewTriangulatedPolygon() error = %v", err)
	}
	if poly.Contains(7, 7) {
		t.Fatal("Contains(7, 7) = true, want false (point is in the L-shape's notch)")
	}
	if !poly.Contains(2, 2) {
		t.Fatal("Contains(2, 2) = false, want true (point is inside the L-shape)")
	}
}

func TestTriangulateRejectsFewerThanThreePoints(t *testing.T) {
	if _, err := Triangulate([]Point{{X: 0, Y: 0}, {X: 1, Y: 1}}); err == nil {
		t.Fatal("Triangulate([2 points]) error = nil, want error")
	}
	if _, err := NewTriangulatedPolygon([]Point{{X: 0, Y: 0}, {X: 1, Y: 1}}); err == nil {
		t.Fatal("NewTriangulatedPolygon([2 points]) error = nil, want error")
	}
}

// TestTriangulateFailsOnPolygonFinishingAtLoopBound locks the ear-clip
// loop-bound semantics: the failure check runs after each ear-clip attempt,
// unconditionally, so a polygon that finishes its triangulation on exactly
// the 100th attempt still fails, the same as one that never converges.
func TestTriangulateFailsOnPolygonFinishingAtLoopBound(t *testing.T) {
	// A convex polygon triangulates in one ear-clip per vertex removed
	// (n - 3 attempts). n=102 finishes in 99 attempts (under the bound);
	// n=103 finishes in exactly 100 (the bound itself, must still fail).
	if _, err := Triangulate(convexPolygon(102)); err != nil {
		t.Fatalf("Triangulate(102-gon) error = %v, want nil (99 ear-clip attempts, under the bound)", err)
	}
	if _, err := Triangulate(convexPolygon(103)); err == nil {
		t.Fatal("Triangulate(103-gon) error = nil, want error (100 ear-clip attempts, exactly the bound)")
	}
}

func convexPolygon(n int) []Point {
	pts := make([]Point, n)
	for i := range pts {
		angle := 2 * math.Pi * float64(i) / float64(n)
		pts[i] = Point{X: int(1000 * math.Cos(angle)), Y: int(1000 * math.Sin(angle))}
	}
	return pts
}

// TestTriangulatedPolygonSizeCountsBothLobes pins the area contract for a
// self-touching ring: the shoelace sum a vertex ring produces cancels the two
// lobes of a bowtie against each other and reports nothing at all, while
// summing triangle sizes reports the area the clipped triangles cover.
// Territory selection is weighted by this number, so the difference decides
// how often a whole territory is picked, not just where inside it.
func TestTriangulatedPolygonSizeCountsBothLobes(t *testing.T) {
	// A bowtie: the corners of a 200x200 box listed so the ring self-crosses.
	points := []Point{
		{X: 0, Y: 0}, {X: 0, Y: 200}, {X: 200, Y: 0}, {X: 200, Y: 200},
	}
	poly, err := NewTriangulatedPolygon(points)
	if err != nil {
		t.Fatalf("NewTriangulatedPolygon() error = %v", err)
	}
	if got := poly.Size(); got != 40000 {
		t.Errorf("Size() = %d, want 40000 (both clipped triangles counted)", got)
	}

	ring, err := NewPolygon(points)
	if err != nil {
		t.Fatalf("NewPolygon() error = %v", err)
	}
	if got := ring.Area(); got != 0 {
		t.Errorf("Polygon.Area() = %v, want 0; the shoelace cancellation this test contrasts against no longer happens", got)
	}
}

// TestTriangulatedPolygonMatchesShippedTerritories pins two territories taken
// verbatim from the shipped spawnlist where the two containment algorithms
// disagree well away from any edge, not merely on the boundary.
func TestTriangulatedPolygonMatchesShippedTerritories(t *testing.T) {
	tests := []struct {
		name   string
		points []Point
		// probe is interior under triangulated containment and outside
		// under even-odd ray casting over the same ring.
		x, y int
	}{
		{
			// spawnlist/23_15.xml "godard01_npc2315_04": four corners of a
			// 200x200 box listed in bowtie order, so the ring self-crosses.
			// The two algorithms cover visibly different halves of the box;
			// this probe sits ~50 units clear of every edge.
			name: "godard01_npc2315_04",
			points: []Point{
				{X: 122400, Y: -69800}, {X: 122400, Y: -70000},
				{X: 122600, Y: -69800}, {X: 122600, Y: -70000},
			},
			x: 122500, y: -69850,
		},
		{
			// spawnlist/23_22.xml "giran17_tb2322_03": a concave hexagon
			// whose last vertex folds back across the shape.
			name: "giran17_tb2322_03",
			points: []Point{
				{X: 117904, Y: 136448}, {X: 103108, Y: 135076},
				{X: 105008, Y: 134864}, {X: 107884, Y: 138588},
				{X: 116032, Y: 139908}, {X: 104556, Y: 139632},
			},
			x: 106500, y: 137000,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			poly, err := NewTriangulatedPolygon(tc.points)
			if err != nil {
				t.Fatalf("NewTriangulatedPolygon() error = %v", err)
			}
			if !poly.Contains(tc.x, tc.y) {
				t.Errorf("Contains(%d, %d) = false, want true", tc.x, tc.y)
			}
			ring, err := NewPolygon(tc.points)
			if err != nil {
				t.Fatalf("NewPolygon() error = %v", err)
			}
			if ring.Contains(tc.x, tc.y) {
				t.Errorf("Polygon.Contains(%d, %d) = true; this fixture is only meaningful "+
					"while the two algorithms disagree at that point", tc.x, tc.y)
			}
		})
	}
}
