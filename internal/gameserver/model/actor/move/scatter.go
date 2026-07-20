package move

import (
	"math/rand/v2"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// RandomNearbyLocation returns a point within offset units of target,
// scattering it randomly when geo confirms the scattered point is directly
// reachable from target and keeping target itself otherwise, then snaps the
// result to ground height. A non-positive offset skips scattering. A nil
// geo returns target unchanged.
func RandomNearbyLocation(geo Geo, target location.Location, offset int) location.Location {
	if geo == nil {
		return target
	}
	if offset > 0 {
		nx := target.X + rand.IntN(2*offset+1) - offset
		ny := target.Y + rand.IntN(2*offset+1) - offset
		if geo.CanMove(target.X, target.Y, target.Z, nx, ny, target.Z) {
			target.X, target.Y = nx, ny
		}
	}
	target.Z = int(geo.Height(target.X, target.Y, target.Z))
	return target
}
