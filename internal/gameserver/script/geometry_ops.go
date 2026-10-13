package script

import (
	"math"
	"slices"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
)

// AngleFrom returns the direction from a to b on the ground plane, in
// degrees from the positive x axis, 0 up to but not including 360.
func AngleFrom(a, b Place) float64 {
	from, to := a.loc(), b.loc()
	angle := math.Atan2(float64(to.Y-from.Y), float64(to.X-from.X)) * (180 / math.Pi)
	if angle < 0 {
		angle += 360
	}
	return angle
}

// InRange reports whether a and b stand within r of each other, body to
// body: r plus both creatures' collision radii, on the ground plane or, with
// includeZ, in 3D. A range of -1 is unlimited; a nil creature is in range
// of nothing.
func InRange(r int, a, b Creature, includeZ bool) bool {
	if a == nil || b == nil {
		return false
	}
	if r == -1 {
		return true
	}
	from, to := locOf(a), locOf(b)
	dx, dy := int64(from.X)-int64(to.X), int64(from.Y)-int64(to.Y)
	dist := dx*dx + dy*dy
	if includeZ {
		dz := int64(from.Z) - int64(to.Z)
		dist += dz * dz
	}
	reach := float64(r) + collisionRadius(a) + collisionRadius(b)
	return float64(dist) <= reach*reach
}

// collisionRadius returns the body radius of c.
func collisionRadius(c Creature) float64 {
	if _, ok := c.(*Door); ok {
		return door.CollisionRadius
	}
	at := combatantOf(c)
	if at == nil {
		panic("script: the body of a handle on nothing")
	}
	return at.CollisionRadius()
}

// Closest returns the position among positions nearest to from in 3D, the
// first listed on a tie; false when there is none.
func Closest(from Place, positions []Loc) (Loc, bool) {
	if len(positions) == 0 {
		return Loc{}, false
	}
	at := from.loc()
	best := positions[0]
	for _, p := range positions[1:] {
		if distance3D(at, p) < distance3D(at, best) {
			best = p
		}
	}
	return best, true
}

// Farthest returns up to count of positions ordered from the farthest from
// from in 3D to the nearest, positions at the same distance in the order
// listed. A Valakas private group's spots are picked this way: the
// reference names the read a closest-position list, but it sorts by
// descending distance.
func Farthest(from Place, positions []Loc, count int) []Loc {
	at := from.loc()
	out := slices.Clone(positions)
	slices.SortStableFunc(out, func(a, b Loc) int {
		da, db := distance3D(at, a), distance3D(at, b)
		switch {
		case da > db:
			return -1
		case da < db:
			return 1
		}
		return 0
	})
	return out[:min(max(count, 0), len(out))]
}

// RandomOffset returns at moved by up to offset on the x and then the y
// axis, each drawn uniformly from -offset to offset through the script
// random source.
func (s *Script) RandomOffset(at Loc, offset int) Loc {
	at.X += s.env.Rand(2*offset+1) - offset
	at.Y += s.env.Rand(2*offset+1) - offset
	return at
}

// Scramble returns the characters of text in a random order, shuffled
// through the script random source: each position from the last down to
// the second swaps with one drawn at or before it.
func (s *Script) Scramble(text string) string {
	chars := []rune(text)
	for i := len(chars); i > 1; i-- {
		j := s.env.Rand(i)
		chars[i-1], chars[j] = chars[j], chars[i-1]
	}
	var sb strings.Builder
	for _, c := range chars {
		sb.WriteRune(c)
	}
	return sb.String()
}
