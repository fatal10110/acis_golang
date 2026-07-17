package geometry

import "testing"

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
