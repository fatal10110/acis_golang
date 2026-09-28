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
	if offset > 0 {
		nx := target.X + rand.IntN(2*offset+1) - offset
		ny := target.Y + rand.IntN(2*offset+1) - offset
		reached := geo.ValidLocation(target.X, target.Y, target.Z, nx, ny, target.Z)
		target.X, target.Y = reached.X, reached.Y
	}
	if keepHeight == nil || !keepHeight(target) {
		target.Z = int(geo.Height(target.X, target.Y, target.Z))
	}
	return target
}
