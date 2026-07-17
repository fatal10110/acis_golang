package zone

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/geometry"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Form is the geometric footprint of a zone: a 3D volume that can answer
// point containment and 2D rectangle overlap (used to attach zones to the
// world's region grid). Forms are immutable once built.
type Form interface {
	// Contains reports whether the point (x, y, z) lies inside the volume.
	Contains(x, y, z int) bool
	// IntersectsRect reports whether the volume's 2D footprint overlaps the
	// axis-aligned rectangle spanning x1..x2 by y1..y2.
	IntersectsRect(x1, x2, y1, y2 int) bool
	// LowZ is the volume's lower z bound.
	LowZ() int
	// HighZ is the volume's upper z bound.
	HighZ() int
}

// zoneForm adapts a geometry.Territory to zone's exact historical
// IntersectsRect contract for a zero-height volume: the reference
// implementation's containment probe for such a volume never reports a
// hit, so a query rectangle sitting entirely inside the footprint is not
// treated as overlapping — only a boundary crossing, or the footprint's
// own vertex landing inside the rectangle, counts.
type zoneForm struct {
	*geometry.Territory
}

func (f zoneForm) IntersectsRect(x1, x2, y1, y2 int) bool {
	if f.LowZ() != f.HighZ() {
		return f.Territory.IntersectsRect(x1, x2, y1, y2)
	}
	for _, s := range f.Shapes {
		if degenerateShapeIntersectsRect(s, x1, x2, y1, y2) {
			return true
		}
	}
	return false
}

// degenerateShapeIntersectsRect reproduces s.IntersectsRect for a
// zero-height volume, omitting the "rectangle corner lies inside the
// shape" containment case a real query never triggers at that height.
func degenerateShapeIntersectsRect(s geometry.Shape, x1, x2, y1, y2 int) bool {
	v, ok := s.(interface{ Vertices() []geometry.Point })
	if !ok {
		// Not vertex-ring backed (a Circle): the reference cylinder form
		// never had this quirk, so the normal test applies unchanged.
		return s.IntersectsRect(x1, x2, y1, y2)
	}
	verts := v.Vertices()
	if len(verts) > 0 {
		fx, fy := verts[0].X, verts[0].Y
		if fx > x1 && fx < x2 && fy > y1 && fy < y2 {
			return true
		}
	}
	rect := []geometry.Point{{X: x1, Y: y1}, {X: x2, Y: y1}, {X: x2, Y: y2}, {X: x1, Y: y2}}
	return geometry.EdgesCross(verts, rect)
}

// NewCuboid builds an axis-aligned box form from two opposite corners;
// coordinates may be given in either order on each axis.
func NewCuboid(x1, x2, y1, y2, z1, z2 int) Form {
	if z1 > z2 {
		z1, z2 = z2, z1
	}
	t, err := geometry.NewTerritory(z1, z2, geometry.NewRectangle(x1, x2, y1, y2))
	if err != nil {
		// Unreachable: z1 <= z2 is enforced above and a Rectangle is
		// always supplied, so NewTerritory's only failure modes can't
		// trigger here.
		panic(err)
	}
	return zoneForm{t}
}

// NewCylinder builds a vertical circular column form centered on (x, y)
// spanning z1..z2. The radius must be positive.
func NewCylinder(x, y, z1, z2, rad int) (Form, error) {
	circle, err := geometry.NewCircle(x, y, rad)
	if err != nil {
		return nil, fmt.Errorf("zone: %w", err)
	}
	t, err := geometry.NewTerritory(z1, z2, circle)
	if err != nil {
		return nil, fmt.Errorf("zone: %w", err)
	}
	return zoneForm{t}, nil
}

// NewPolygon builds a vertical-prism form over an arbitrary simple polygon
// from at least three vertices spanning z1..z2.
func NewPolygon(nodes []location.Point, z1, z2 int) (Form, error) {
	if len(nodes) < 3 {
		return nil, fmt.Errorf("zone: polygon needs at least 3 vertices, got %d", len(nodes))
	}
	points := make([]geometry.Point, len(nodes))
	for i, n := range nodes {
		points[i] = geometry.Point{X: n.X, Y: n.Y}
	}
	poly, err := geometry.NewPolygon(points)
	if err != nil {
		return nil, fmt.Errorf("zone: %w", err)
	}
	t, err := geometry.NewTerritory(z1, z2, poly)
	if err != nil {
		return nil, fmt.Errorf("zone: %w", err)
	}
	return zoneForm{t}, nil
}
