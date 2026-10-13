package network

import "github.com/fatal10110/acis_golang/internal/gameserver/model/location"

type sequentialIDs struct{ next int32 }

func (s *sequentialIDs) NextID() (int32, error) {
	s.next++
	return s.next, nil
}

// testGeo is an always-passable move.Geo double for tests that don't
// exercise geodata behavior.
type testGeo struct{}

func (testGeo) CanMove(int, int, int, int, int, int) bool { return true }
func (testGeo) Height(_, _, z int) int16                  { return int16(z) }

func (testGeo) FindPath(_, _ location.Location) ([]location.Location, bool) { return nil, false }
func (testGeo) Walkable(int, int, int) bool                                 { return true }

func (g testGeo) CanFly(ox, oy, oz int, _ float64, tx, ty, tz int) bool {
	return g.CanMove(ox, oy, oz, tx, ty, tz)
}

func (g testGeo) ValidFlyLocation(ox, oy, oz int, _ float64, tx, ty, tz int) location.Location {
	return g.ValidLocation(ox, oy, oz, tx, ty, tz)
}

func (testGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}
