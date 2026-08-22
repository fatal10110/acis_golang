package move

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

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
	heightCalls        []location.Location
	moveCalls          []geoCall
	findPathCalls      []findPathCall
	validLocationCalls []validLocationCall
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

// fakeMoveClock/fakeMoveTimer are the sanctioned clock/timer infra-seam
// exception (docs/agents/test-strategy.md): determinism is the point, not
// integration coverage. Kept as-is.
type fakeMoveClock struct {
	timers []*fakeMoveTimer
}

func (c *fakeMoveClock) AfterFunc(delay time.Duration, f func()) scheduledTimer {
	timer := &fakeMoveTimer{delay: delay, f: f}
	c.timers = append(c.timers, timer)
	return timer
}

// fire runs every still-pending timer, latest scheduled first, so a
// superseded earlier timer (already Stop()ped by the newer request) is
// correctly skipped even though both share the same delay.
func (c *fakeMoveClock) fire(delay time.Duration) {
	for i := len(c.timers) - 1; i >= 0; i-- {
		timer := c.timers[i]
		if timer.delay == delay && !timer.stopped {
			timer.stopped = true
			timer.f()
		}
	}
}

type fakeMoveTimer struct {
	delay   time.Duration
	f       func()
	stopped bool
}

func (t *fakeMoveTimer) Stop() bool {
	if t.stopped {
		return false
	}
	t.stopped = true
	return true
}

// noAllocTimer is a zero-size scheduledTimer: converting a zero-width value
// to an interface does not allocate, so installing it as afterFunc isolates
// FollowTick's own allocation profile from the real runtime timer's.
type noAllocTimer struct{}

func (noAllocTimer) Stop() bool { return true }
