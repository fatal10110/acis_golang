package geometry

import "testing"

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

func TestTriangleArea(t *testing.T) {
	tri, err := NewTriangle(Point{0, 0}, Point{10, 0}, Point{0, 10})
	if err != nil {
		t.Fatalf("NewTriangle: %v", err)
	}
	if got := tri.Area(); got != 50 {
		t.Errorf("Area() = %v, want 50", got)
	}
}
