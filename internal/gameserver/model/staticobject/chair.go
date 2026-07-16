package staticobject

import "github.com/fatal10110/acis_golang/internal/gameserver/model/location"

const (
	// ChairType is the static-object type used by sit-capable chairs.
	ChairType = 1
	// ChairInteractionDistance is the maximum distance for sitting on a chair.
	ChairInteractionDistance = 150
)

// ChairUser is an actor that may try to sit on a static-object chair.
type ChairUser interface {
	Position() (int, int, int)
	AlikeDead() bool
	Standing() bool
}

// Chair is a static object that can be claimed for sitting.
type Chair interface {
	Position() (int, int, int)
	Type() int
	SetBusy(bool) bool
}

// ClaimChair marks chair busy when user can sit on it.
func ClaimChair(user ChairUser, chair Chair, radius int) bool {
	if user == nil || chair == nil || user.AlikeDead() || !user.Standing() {
		return false
	}
	if chair.Type() != ChairType || !inRange(user, chair, radius) {
		return false
	}
	return chair.SetBusy(true)
}

func inRange(a, b interface{ Position() (int, int, int) }, radius int) bool {
	ax, ay, az := a.Position()
	bx, by, bz := b.Position()
	return location.In3DRange(ax, ay, az, bx, by, bz, radius)
}
