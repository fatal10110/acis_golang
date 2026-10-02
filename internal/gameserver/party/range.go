package party

// Body is a creature measured for party range: its position and its
// collision radius.
type Body interface {
	Position() (x, y, z int)
	CollisionRadius() float64
}

// InRange reports whether a and b are within partyRange of each other,
// body to body in 3D: the range plus both collision radii. A range of -1
// is unlimited.
func InRange(partyRange int, a, b Body) bool {
	if partyRange == -1 {
		return true
	}
	ax, ay, az := a.Position()
	bx, by, bz := b.Position()
	dx, dy, dz := int64(ax-bx), int64(ay-by), int64(az-bz)
	reach := float64(partyRange) + a.CollisionRadius() + b.CollisionRadius()
	return float64(dx*dx+dy*dy+dz*dz) <= reach*reach
}
