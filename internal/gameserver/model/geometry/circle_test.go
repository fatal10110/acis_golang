package geometry

import (
	"math"
	"testing"
)

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
