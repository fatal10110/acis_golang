package geometry

import (
	"fmt"
	"math"
)

// Circle is a 2D disc.
type Circle struct {
	x, y, rad int
	radSq     int64
}

// NewCircle builds a Circle centered on (x, y). The radius must be
// positive.
func NewCircle(x, y, rad int) (Circle, error) {
	if rad <= 0 {
		return Circle{}, fmt.Errorf("geometry: circle radius must be positive, got %d", rad)
	}
	return Circle{x: x, y: y, rad: rad, radSq: int64(rad) * int64(rad)}, nil
}

// Contains reports whether (x, y) lies inside the disc: squared planar
// distance at most radius squared. Integer squared distances stay well
// below 2^53, so this matches a double-precision evaluation exactly.
func (c Circle) Contains(x, y int) bool {
	dx, dy := int64(c.x-x), int64(c.y-y)
	return dx*dx+dy*dy <= c.radSq
}

// Area is the disc's area.
func (c Circle) Area() float64 { return math.Pi * float64(c.rad) * float64(c.rad) }

// IntersectsRect reports whether the disc overlaps the rectangle. Corner
// and side probes are strict: a circle exactly tangent to a rectangle side
// does not count as overlapping.
func (c Circle) IntersectsRect(ax1, ax2, ay1, ay2 int) bool {
	// Center strictly inside the rectangle?
	if c.x > ax1 && c.x < ax2 && c.y > ay1 && c.y < ay2 {
		return true
	}
	// Any rectangle corner strictly inside the circle?
	dist := func(px, py int) int64 {
		dx, dy := int64(px-c.x), int64(py-c.y)
		return dx*dx + dy*dy
	}
	if dist(ax1, ay1) < c.radSq || dist(ax1, ay2) < c.radSq || dist(ax2, ay1) < c.radSq || dist(ax2, ay2) < c.radSq {
		return true
	}
	// Circle crossing a side of the rectangle?
	abs := func(v int) int {
		if v < 0 {
			return -v
		}
		return v
	}
	if c.x > ax1 && c.x < ax2 {
		if abs(c.y-ay2) < c.rad || abs(c.y-ay1) < c.rad {
			return true
		}
	}
	if c.y > ay1 && c.y < ay2 {
		if abs(c.x-ax2) < c.rad || abs(c.x-ax1) < c.rad {
			return true
		}
	}
	return false
}

// Intersects reports whether c overlaps other, dispatching on other's kind.
func (c Circle) Intersects(other Shape) bool { return intersects(c, other) }
