package geometry

import "errors"

// Territory is a 3D region: a vertical z range paired with one or more 2D
// shapes whose union describes the footprint at every height in that
// range.
type Territory struct {
	MinZ, MaxZ int
	Shapes     []Shape
}

// NewTerritory builds a Territory spanning minZ..maxZ over the union of
// shapes. At least one shape is required. minZ may exceed maxZ (some
// source data declares inverted ranges); Contains then simply never
// matches any z, rather than NewTerritory rejecting the input.
func NewTerritory(minZ, maxZ int, shapes ...Shape) (*Territory, error) {
	if len(shapes) == 0 {
		return nil, errors.New("geometry: territory needs at least one shape")
	}
	return &Territory{MinZ: minZ, MaxZ: maxZ, Shapes: shapes}, nil
}

// Contains reports whether (x, y, z) lies inside the territory: z within
// [MinZ, MaxZ], and (x, y) inside any one of its shapes (union semantics
// across shapes).
func (t *Territory) Contains(x, y, z int) bool {
	if z < t.MinZ || z > t.MaxZ {
		return false
	}
	for _, s := range t.Shapes {
		if s.Contains(x, y) {
			return true
		}
	}
	return false
}

// IntersectsRect reports whether the territory's 2D footprint overlaps the
// axis-aligned rectangle spanning x1..x2 by y1..y2.
func (t *Territory) IntersectsRect(x1, x2, y1, y2 int) bool {
	for _, s := range t.Shapes {
		if s.IntersectsRect(x1, x2, y1, y2) {
			return true
		}
	}
	return false
}

// LowZ is the territory's lower z bound.
func (t *Territory) LowZ() int { return t.MinZ }

// HighZ is the territory's upper z bound.
func (t *Territory) HighZ() int { return t.MaxZ }

// Area sums the area of every shape. Overlap between shapes in the same
// territory is not deduplicated — no current caller needs it.
func (t *Territory) Area() float64 {
	var sum float64
	for _, s := range t.Shapes {
		sum += s.Area()
	}
	return sum
}

// Intersects reports whether t and o overlap as 3D volumes: their z ranges
// overlap, and some shape of t overlaps some shape of o.
func (t *Territory) Intersects(o *Territory) bool {
	if t.MaxZ < o.MinZ || o.MaxZ < t.MinZ {
		return false
	}
	for _, s := range t.Shapes {
		for _, os := range o.Shapes {
			if s.Intersects(os) {
				return true
			}
		}
	}
	return false
}
