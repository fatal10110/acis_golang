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
// positive, and the centre and radius must fit in 32 bits: that bound keeps
// every squared distance and bounding-box edge clear of int64 overflow.
func NewCircle(x, y, rad int) (Circle, error) {
	if rad <= 0 {
		return Circle{}, fmt.Errorf("geometry: circle radius must be positive, got %d", rad)
	}
	if rad > math.MaxInt32 {
		return Circle{}, fmt.Errorf("geometry: circle radius %d exceeds the 32-bit range", rad)
	}
	if !fitsInt32(x) || !fitsInt32(y) {
		return Circle{}, fmt.Errorf("geometry: circle centre (%d, %d) is outside the 32-bit range", x, y)
	}
	return Circle{x: x, y: y, rad: rad, radSq: int64(rad) * int64(rad)}, nil
}

func fitsInt32(v int) bool { return v >= math.MinInt32 && v <= math.MaxInt32 }

// sqDist returns the squared distance from the centre to (px, py), or
// math.MaxInt64 when the point lies more than the radius away on either
// axis. Such a point is outside the disc, and skipping the squares there
// keeps them below int64 overflow for any centre and point in the 32-bit
// range.
func (c Circle) sqDist(px, py int) int64 {
	dx, dy, r := int64(px-c.x), int64(py-c.y), int64(c.rad)
	if dx < -r || dx > r || dy < -r || dy > r {
		return math.MaxInt64
	}
	return dx*dx + dy*dy
}

// Contains reports whether (x, y) lies inside the disc: squared planar
// distance at most radius squared. Integer squared distances stay well
// below 2^53 for world coordinates, so this matches a double-precision
// evaluation exactly.
func (c Circle) Contains(x, y int) bool { return c.sqDist(x, y) <= c.radSq }

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
	if c.sqDist(ax1, ay1) < c.radSq || c.sqDist(ax1, ay2) < c.radSq || c.sqDist(ax2, ay1) < c.radSq || c.sqDist(ax2, ay2) < c.radSq {
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

// Bounds is the disc's bounding box.
func (c Circle) Bounds() (minX, maxX, minY, maxY int) {
	return c.x - c.rad, c.x + c.rad, c.y - c.rad, c.y + c.rad
}

// Intersects reports whether c overlaps other, dispatching on other's kind.
func (c Circle) Intersects(other Shape) bool { return intersects(c, other) }
