package move

import (
	"math/rand/v2"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// RandomNearbyLocation resolves a teleport destination near target. A
// positive offset scatters target by up to offset units on each axis and
// clips the scatter to the last point geo can reach on the straight line
// from target, walked at target's own height. The result's Z then snaps to
// ground height unless keepHeight (nil means never) reports that the
// destination keeps its own height, as a flying creature's or one inside
// water does. A nil geo returns target unchanged.
func RandomNearbyLocation(geo Geo, target location.Location, offset int, keepHeight func(location.Location) bool) location.Location {
	if geo == nil {
		return target
	}
	j := location.Jump{To: target}
	if offset > 0 {
		nx := target.X + rand.IntN(2*offset+1) - offset
		ny := target.Y + rand.IntN(2*offset+1) - offset
		j.Toward, j.Scattered = location.Location{X: nx, Y: ny, Z: target.Z}, true
	}
	return Land(geo, j, keepHeight)
}

// LandingGeo is the geodata a jump lands through.
type LandingGeo interface {
	ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location
	Height(x, y, z int) int16
}

// Land returns where j lands: a scattered jump stops at the last point geo
// can reach on the straight line from j.To toward j.Toward, walked at
// j.To's height; the result's Z then snaps to ground height unless
// keepHeight (nil means never) reports that the point keeps its own
// height, as a flying creature's or one inside water does. A nil geo lands
// at j.To.
func Land(geo LandingGeo, j location.Jump, keepHeight func(location.Location) bool) location.Location {
	at := j.To
	if geo == nil {
		return at
	}
	if j.Scattered {
		reached := geo.ValidLocation(at.X, at.Y, at.Z, j.Toward.X, j.Toward.Y, at.Z)
		at.X, at.Y = reached.X, reached.Y
	}
	if keepHeight == nil || !keepHeight(at) {
		at.Z = int(geo.Height(at.X, at.Y, at.Z))
	}
	return at
}
