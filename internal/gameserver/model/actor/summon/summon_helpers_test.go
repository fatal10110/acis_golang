package summon

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// ---- from conditions_test.go ----
// openGeo allows every straight-line move and echoes back requested
// heights/targets unmodified, enough to drive CreatureMove.MoveToLocation
// deterministically without real geodata.
type openGeo struct{}

func (openGeo) CanMove(ox, oy, oz, tx, ty, tz int) bool { return true }
func (openGeo) Height(x, y, z int) int16                { return int16(z) }
func (openGeo) FindPath(origin, target location.Location) ([]location.Location, bool) {
	return nil, false
}
func (openGeo) Walkable(int, int, int) bool { return true }

func (g openGeo) CanFly(ox, oy, oz int, _ float64, tx, ty, tz int) bool {
	return g.CanMove(ox, oy, oz, tx, ty, tz)
}

func (g openGeo) ValidFlyLocation(ox, oy, oz int, _ float64, tx, ty, tz int) location.Location {
	return g.ValidLocation(ox, oy, oz, tx, ty, tz)
}

func (openGeo) ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location {
	return location.Location{X: tx, Y: ty, Z: tz}
}

func zeroSummonRoll(int) int { return 0 }
