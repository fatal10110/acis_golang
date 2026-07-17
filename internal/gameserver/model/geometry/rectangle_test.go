package geometry

import "testing"

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
