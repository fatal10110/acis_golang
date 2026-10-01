package move

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

type homeRecoverySelf struct {
	playerFollowSelf
	failCount int
	teleports []location.Location
}

func (s *homeRecoverySelf) GeoPathFailCount() int  { return s.failCount }
func (s *homeRecoverySelf) ResetGeoPathFailCount() { s.failCount = 0 }
func (s *homeRecoverySelf) AddGeoPathFailCount()   { s.failCount++ }
func (s *homeRecoverySelf) TeleportTo(loc location.Location) {
	s.teleports = append(s.teleports, loc)
	s.SyncPosition(loc)
}

func TestControllerMoveHomeTeleportsAfterTenBlockedPaths(t *testing.T) {
	self := &homeRecoverySelf{}
	geo := &recordingGeo{canMove: false, height: 0, findPathOK: false}
	mover, err := NewCreatureMove(location.Location{X: 100, Y: 100, Z: 0}, 100, geo)
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)
	controller, err := NewController(mover, self, nil)
	if err != nil {
		t.Fatal(err)
	}
	home := location.Location{X: 0, Y: 0, Z: 0}

	for range HomeGeoFailLimit {
		if err := controller.MoveHome(home); err != nil {
			t.Fatalf("MoveHome() error = %v", err)
		}
	}
	if got := self.failCount; got != HomeGeoFailLimit {
		t.Fatalf("failCount = %d, want %d", got, HomeGeoFailLimit)
	}
	if len(self.teleports) != 0 {
		t.Fatal("teleport before fail limit")
	}

	if err := controller.MoveHome(home); err != nil {
		t.Fatalf("MoveHome() after limit error = %v", err)
	}
	if len(self.teleports) != 1 || self.teleports[0] != home {
		t.Fatalf("teleports = %+v, want [%+v]", self.teleports, home)
	}
	if got := self.failCount; got != 0 {
		t.Fatalf("failCount after teleport = %d, want 0", got)
	}
	x, y, z := self.Position()
	if got := (location.Location{X: x, Y: y, Z: z}); got != home {
		t.Fatalf("Position() = %+v, want teleported home %+v", got, home)
	}
}

func TestControllerMoveHomeReturnsMoveErrors(t *testing.T) {
	self := &homeRecoverySelf{}
	mover, err := NewCreatureMove(location.Location{}, 0, staticGeo{canMove: true})
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)
	controller, err := NewController(mover, self, nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := controller.MoveHome(location.Location{X: 100}); err == nil {
		t.Fatal("MoveHome() error = nil, want zero-speed rejection")
	}
	if got := self.failCount; got != 0 {
		t.Fatalf("failCount = %d, want 0: a rejected move never pathfinds", got)
	}
}

func TestControllerMoveHomeResetsFailCountOnRoutedPath(t *testing.T) {
	self := &homeRecoverySelf{failCount: 5}
	waypoints := []location.Location{{X: 50, Y: 0, Z: 0}, {X: 100, Y: 0, Z: 0}}
	geo := &recordingGeo{
		canMove:    false,
		height:     0,
		findPath:   waypoints,
		findPathOK: true,
	}
	mover, err := NewCreatureMove(location.Location{}, 100, geo)
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)
	controller, err := NewController(mover, self, nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := controller.MoveHome(location.Location{X: 100, Y: 0, Z: 0}); err != nil {
		t.Fatalf("MoveHome() error = %v", err)
	}
	if got := self.failCount; got != 0 {
		t.Fatalf("failCount = %d, want 0 after routed path", got)
	}
}

func TestControllerMoveToLocationIncrementsFailCountOnBlockedPath(t *testing.T) {
	self := &homeRecoverySelf{}
	geo := &recordingGeo{canMove: false, height: 0, findPathOK: false}
	mover, err := NewCreatureMove(location.Location{X: 100, Y: 100, Z: 0}, 100, geo)
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)
	controller, err := NewController(mover, self, nil)
	if err != nil {
		t.Fatal(err)
	}

	accepted, err := controller.MoveToLocation(location.Location{X: 0, Y: 0, Z: 0})
	if err != nil {
		t.Fatalf("MoveToLocation() error = %v", err)
	}
	if !accepted {
		t.Fatal("MoveToLocation() = false, want accepted fallback walk")
	}
	if got := self.failCount; got != 1 {
		t.Fatalf("failCount = %d, want 1 after blocked pathfinding move", got)
	}
}

func TestControllerMoveToLocationResetsFailCountOnRoutedPath(t *testing.T) {
	self := &homeRecoverySelf{failCount: 5}
	waypoints := []location.Location{{X: 50, Y: 0, Z: 0}, {X: 100, Y: 0, Z: 0}}
	geo := &recordingGeo{
		canMove:    false,
		height:     0,
		findPath:   waypoints,
		findPathOK: true,
	}
	mover, err := NewCreatureMove(location.Location{}, 100, geo)
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)
	controller, err := NewController(mover, self, nil)
	if err != nil {
		t.Fatal(err)
	}

	accepted, err := controller.MoveToLocation(location.Location{X: 100, Y: 0, Z: 0})
	if err != nil {
		t.Fatalf("MoveToLocation() error = %v", err)
	}
	if !accepted {
		t.Fatal("MoveToLocation() = false, want accepted routed walk")
	}
	if got := self.failCount; got != 0 {
		t.Fatalf("failCount = %d, want 0 after routed pathfinding move", got)
	}
}

func TestControllerMoveToLocationLeavesFailCountOnDirectPath(t *testing.T) {
	self := &homeRecoverySelf{failCount: 5}
	mover, err := NewCreatureMove(location.Location{}, 100, staticGeo{canMove: true})
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)
	controller, err := NewController(mover, self, nil)
	if err != nil {
		t.Fatal(err)
	}

	accepted, err := controller.MoveToLocation(location.Location{X: 100, Y: 0, Z: 0})
	if err != nil {
		t.Fatalf("MoveToLocation() error = %v", err)
	}
	if !accepted {
		t.Fatal("MoveToLocation() = false, want accepted direct walk")
	}
	if got := self.failCount; got != 5 {
		t.Fatalf("failCount = %d, want 5 after straight-line move", got)
	}
}

func TestControllerMoveToLocationEventIncrementsFailCountOnBlockedPath(t *testing.T) {
	self := &homeRecoverySelf{}
	geo := &recordingGeo{canMove: false, height: 0, findPathOK: false}
	mover, err := NewCreatureMove(location.Location{X: 100, Y: 100, Z: 0}, 100, geo)
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)
	controller, err := NewController(mover, self, nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := controller.MoveToLocationEvent(location.Location{X: 0, Y: 0, Z: 0}); err != nil {
		t.Fatalf("MoveToLocationEvent() error = %v", err)
	}
	if got := self.failCount; got != 1 {
		t.Fatalf("failCount = %d, want 1 after blocked walker pathfinding move", got)
	}
}

// npcHomeRecoverySelf is a hostile NPC: it recovers stalled home paths and
// approaches its target with a plain walk.
type npcHomeRecoverySelf struct{ homeRecoverySelf }

func (*npcHomeRecoverySelf) OffensiveFollowIsPawnMove() bool { return false }

func TestControllerOffensiveFollowIncrementsFailCountOnBlockedPath(t *testing.T) {
	self := &npcHomeRecoverySelf{}
	geo := &recordingGeo{canMove: false, height: 0, findPathOK: false}
	mover, err := NewCreatureMove(location.Location{}, 100, geo)
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)
	controller, err := NewController(mover, self, nil)
	if err != nil {
		t.Fatal(err)
	}

	following, err := controller.MaybeStartOffensiveFollow(&followTarget{x: 200}, 40)
	if err != nil {
		t.Fatal(err)
	}
	if !following {
		t.Fatal("MaybeStartOffensiveFollow() = false, want true when target is out of range")
	}
	if got := self.failCount; got != 1 {
		t.Fatalf("failCount = %d, want 1 after blocked offensive-follow pathfinding", got)
	}
}
