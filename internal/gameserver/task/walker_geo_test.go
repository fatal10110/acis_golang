package task

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

var (
	walkerNodeA = location.Location{X: 0, Y: 0, Z: 0}
	walkerNodeB = location.Location{X: 100, Y: 0, Z: 0}
	walkerNodeC = location.Location{X: 200, Y: 0, Z: 0}
)

// wrapBlockedPath fails reachability only toward blocked, so a wrap from the
// last node onto that node increments and rewinds to the previous node.
type wrapBlockedPath struct {
	blocked location.Location
}

func (p wrapBlockedPath) CanMove(_, target location.Location) bool {
	return target != p.blocked
}

func (p wrapBlockedPath) HasPath(_, target location.Location) bool {
	return target != p.blocked
}

type routedGeo struct{}

func (routedGeo) CanMove(_, _, _, _, _, _ int) bool { return false }

func (routedGeo) Height(_, _, z int) int16 { return int16(z) }

func (routedGeo) FindPath(_, target location.Location) ([]location.Location, bool) {
	return []location.Location{{X: (target.X + 200) / 2, Y: target.Y, Z: target.Z}, target}, true
}

func (routedGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

func (routedGeo) Walkable(int, int, int) bool { return true }

type walkerCtlSelf struct {
	x, y, z   int
	failCount int
	adds      int
}

func (s *walkerCtlSelf) ObjectID() int32                    { return 1 }
func (s *walkerCtlSelf) Position() (int, int, int)          { return s.x, s.y, s.z }
func (s *walkerCtlSelf) CollisionRadius() float64           { return 0 }
func (s *walkerCtlSelf) SetHeading(int)                     {}
func (s *walkerCtlSelf) SyncPosition(pos location.Location) { s.x, s.y, s.z = pos.X, pos.Y, pos.Z }
func (s *walkerCtlSelf) BroadcastMove(move.Event) error     { return nil }
func (s *walkerCtlSelf) BroadcastStop() error               { return nil }
func (s *walkerCtlSelf) GeoPathFailCount() int {
	return s.failCount
}
func (s *walkerCtlSelf) ResetGeoPathFailCount() { s.failCount = 0 }
func (s *walkerCtlSelf) AddGeoPathFailCount() {
	s.failCount++
	s.adds++
}
func (s *walkerCtlSelf) TeleportTo(loc location.Location) {
	s.SyncPosition(loc)
}

type controllerWalker struct {
	world.Presence
	self *walkerCtlSelf
	ctl  *move.Controller
}

func (w *controllerWalker) ObjectID() int32 { return w.self.ObjectID() }

func (w *controllerWalker) Position() location.Location {
	x, y, z := w.self.Position()
	return location.Location{X: x, Y: y, Z: z}
}

func (w *controllerWalker) Moving() bool { return false }

func (w *controllerWalker) MoveToLocation(target location.Location) (move.Event, error) {
	return w.ctl.MoveToLocationEvent(target)
}

func (w *controllerWalker) TeleportTo(target location.Location) { w.self.TeleportTo(target) }
func (w *controllerWalker) GeoPathFailCount() int               { return w.self.GeoPathFailCount() }
func (w *controllerWalker) ResetGeoPathFailCount()              { w.self.ResetGeoPathFailCount() }
func (w *controllerWalker) AddGeoPathFailCount()                { w.self.AddGeoPathFailCount() }
func (w *controllerWalker) SayNPCString(int)                    {}
func (w *controllerWalker) SocialAction(int)                    {}

func TestWalkerRoutedFallbackClearsGeoPathFailCount(t *testing.T) {
	origin := walkerNodeC
	self := &walkerCtlSelf{x: origin.X, y: origin.Y, z: origin.Z}
	mover, err := move.NewCreatureMove(origin, 100, routedGeo{})
	if err != nil {
		t.Fatal(err)
	}
	ctl, err := move.NewController(mover, self)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ctl.Stop() })
	actor := &controllerWalker{self: self, ctl: ctl}

	routes := route.WalkerRoutes{
		"r": {"n": {
			{Location: walkerNodeA},
			{Location: walkerNodeB},
			{Location: walkerNodeC},
		}},
	}
	w, err := NewWalker(routes, wrapBlockedPath{blocked: walkerNodeA}, time.Now, nil)
	if err != nil {
		t.Fatalf("NewWalker() error: %v", err)
	}

	if err := w.StartRoute(actor, "r", "n"); err != nil {
		t.Fatalf("StartRoute() error: %v", err)
	}
	if got := actor.GeoPathFailCount(); got != 0 {
		t.Fatalf("GeoPathFailCount() after start = %d, want 0", got)
	}

	if err := w.MoveToNextPoint(actor); err != nil {
		t.Fatalf("MoveToNextPoint() error: %v", err)
	}
	if got := self.adds; got != 1 {
		t.Fatalf("AddGeoPathFailCount calls = %d, want 1 from the blocked wrap onto A", got)
	}
	if got := actor.GeoPathFailCount(); got != 0 {
		t.Fatalf("GeoPathFailCount() after blocked wrap + routed fallback = %d, want 0", got)
	}
}
