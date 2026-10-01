package move

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable/attackabletest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// ---- from controller_3d_follow_test.go ----
type playerFollowSelf struct {
	x, y, z  int
	moves    []event.Move
	disabled bool
}

type tickerOwnedFollowSelf struct{ playerFollowSelf }

func (*tickerOwnedFollowSelf) OwnsOffensiveFollowTicker() bool { return true }

func (s *playerFollowSelf) followSelf() *playerFollowSelf      { return s }
func (s *playerFollowSelf) ObjectID() int32                    { return 1 }
func (s *playerFollowSelf) Position() (int, int, int)          { return s.x, s.y, s.z }
func (s *playerFollowSelf) CollisionRadius() float64           { return 0 }
func (s *playerFollowSelf) SetHeading(int)                     {}
func (s *playerFollowSelf) SyncPosition(pos location.Location) { s.x, s.y, s.z = pos.X, pos.Y, pos.Z }

func (s *playerFollowSelf) BroadcastMove(ev event.Move) {
	s.moves = append(s.moves, ev)
}
func (s *playerFollowSelf) BroadcastStop()                  {}
func (s *playerFollowSelf) OffensiveFollowIsPawnMove() bool { return true }

type followTarget struct {
	attackabletest.Combatant
	x, y, z int
	moving  bool
}

func (t *followTarget) ObjectID() int32           { return 2 }
func (t *followTarget) SiegeGuard() bool          { return false }
func (t *followTarget) AlikeDead() bool           { return false }
func (t *followTarget) Position() (int, int, int) { return t.x, t.y, t.z }
func (t *followTarget) CollisionRadius() float64  { return 0 }
func (t *followTarget) IsMoving() bool            { return t.moving }

var _ attackable.Combatant = (*followTarget)(nil)

// ---- from creature_fakes_test.go ----
type geoCall struct {
	origin, target location.Location
}

type findPathCall struct {
	origin, target location.Location
}

type validLocationCall struct {
	origin, target location.Location
}

// recordingGeo stands in for the geo/pathfind boundary; per
// docs/agents/test-strategy.md's pure-algorithm/no-boundary exception,
// EngineGeo needs a real geodata engine and pathfinder to construct, which
// is disproportionate for these move-resolution unit tests. Kept as-is.
type recordingGeo struct {
	canMove            bool
	canMoveAt          func(ox, oy, oz, tx, ty, tz int) bool
	height             int16
	findPath           []location.Location
	findPathOK         bool
	validLocation      location.Location
	canFly             bool
	validFlyLocation   location.Location
	heightCalls        []location.Location
	moveCalls          []geoCall
	findPathCalls      []findPathCall
	validLocationCalls []validLocationCall
	flyCalls           []flyCall
	validFlyCalls      []flyCall
}

// flyCall is one fly query: the line and the corridor height asked for.
type flyCall struct {
	origin, target location.Location
	height         float64
}

func (g *recordingGeo) CanMove(ox, oy, oz, tx, ty, tz int) bool {
	g.moveCalls = append(g.moveCalls, geoCall{
		origin: location.Location{X: ox, Y: oy, Z: oz},
		target: location.Location{X: tx, Y: ty, Z: tz},
	})
	if g.canMoveAt != nil {
		return g.canMoveAt(ox, oy, oz, tx, ty, tz)
	}
	return g.canMove
}

func (g *recordingGeo) Height(x, y, z int) int16 {
	g.heightCalls = append(g.heightCalls, location.Location{X: x, Y: y, Z: z})
	return g.height
}

func (g *recordingGeo) FindPath(origin, target location.Location) ([]location.Location, bool) {
	g.findPathCalls = append(g.findPathCalls, findPathCall{origin: origin, target: target})
	return g.findPath, g.findPathOK
}

func (g *recordingGeo) ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location {
	g.validLocationCalls = append(g.validLocationCalls, validLocationCall{
		origin: location.Location{X: ox, Y: oy, Z: oz},
		target: location.Location{X: tx, Y: ty, Z: tz},
	})
	// Unset means "no progress", mirroring the real engine's same-cell fallback.
	if g.validLocation == (location.Location{}) {
		return location.Location{X: ox, Y: oy, Z: oz}
	}
	return g.validLocation
}

func (g *recordingGeo) Walkable(int, int, int) bool { return true }

func (g *recordingGeo) CanFly(ox, oy, oz int, oheight float64, tx, ty, tz int) bool {
	g.flyCalls = append(g.flyCalls, flyCall{
		origin: location.Location{X: ox, Y: oy, Z: oz},
		target: location.Location{X: tx, Y: ty, Z: tz},
		height: oheight,
	})
	return g.canFly
}

func (g *recordingGeo) ValidFlyLocation(ox, oy, oz int, oheight float64, tx, ty, tz int) location.Location {
	g.validFlyCalls = append(g.validFlyCalls, flyCall{
		origin: location.Location{X: ox, Y: oy, Z: oz},
		target: location.Location{X: tx, Y: ty, Z: tz},
		height: oheight,
	})
	if g.validFlyLocation == (location.Location{}) {
		return location.Location{X: ox, Y: oy, Z: oz}
	}
	return g.validFlyLocation
}

// staticGeo is a zero-allocation Geo stub for allocation-ceiling tests:
// recordingGeo's call-log slices grow and occasionally reallocate, which
// would add noise to a per-call allocation measurement.
type staticGeo struct {
	canMove bool
	height  int16
}

func (g staticGeo) CanMove(ox, oy, oz, tx, ty, tz int) bool { return g.canMove }

func (g staticGeo) Height(x, y, z int) int16 { return g.height }

// staticGeo never reports a found path or partial-progress fall-back: it
// models terrain that has either an open line (canMove=true) or an absolute
// block (canMove=false), whose fallback is a zero-distance arrival.
func (g staticGeo) FindPath(_, _ location.Location) ([]location.Location, bool) { return nil, false }

func (g staticGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

func (g staticGeo) Walkable(int, int, int) bool { return true }

func (g staticGeo) CanFly(int, int, int, float64, int, int, int) bool { return g.canMove }

func (g staticGeo) ValidFlyLocation(ox, oy, oz int, _ float64, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

// moveClock runs a mover's queue on a virtual clock that moves only on
// Advance, so an arrival timer fires only when a test lets it.
type moveClock struct {
	in *sim.Inline
	q  *sim.Queue
}

func newMoveClock() *moveClock {
	in := sim.NewInline(time.Unix(1000, 0))
	return &moveClock{in: in, q: in.NewQueue("mover")}
}

// hookOwner adapts test callbacks to the moveOwner milestones.
type hookOwner struct {
	onArrived, onBlocked func()
	onAdvanced           func(event.Move)
}

func (o *hookOwner) arrived() {
	if o.onArrived != nil {
		o.onArrived()
	}
}

func (o *hookOwner) blocked() {
	if o.onBlocked != nil {
		o.onBlocked()
	}
}

func (*hookOwner) pawnStepped(location.Location, location.Location) {}

func (*hookOwner) knowsPawn(Pawn) bool { return true }

func (o *hookOwner) segmentAdvanced(ev event.Move) {
	if o.onAdvanced != nil {
		o.onAdvanced(ev)
	}
}

func (followTarget) Kind() actor.Kind { return actor.KindNPC }

func (followTarget) Heading() int { return 0 }

func (*playerFollowSelf) OwnsOffensiveFollowTicker() bool { return false }
func (s *playerFollowSelf) MovementDisabled() bool        { return s.disabled }
