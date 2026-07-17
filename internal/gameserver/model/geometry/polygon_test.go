package geometry

import "testing"

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
