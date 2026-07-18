package task

import (
	"errors"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
)

func TestWalkerStartRouteUsesNearestNodeAndAdvances(t *testing.T) {
	routes := route.WalkerRoutes{
		"patrol": {
			"guard": {walkerNode(0), walkerNode(100), walkerNode(200)},
		},
	}
	actor := &walkerActorStub{id: 1, pos: loc(140)}
	w := newTestWalker(t, routes, &walkerPathStub{canMove: true}, nil)

	if err := w.StartRoute(actor, "patrol", "guard"); err != nil {
		t.Fatal(err)
	}
	assertMoves(t, actor, loc(100))

	actor.pos = loc(100)
	if err := w.Arrived(actor); err != nil {
		t.Fatal(err)
	}
	assertMoves(t, actor, loc(100), loc(200))

	actor.pos = loc(200)
	if err := w.Arrived(actor); err != nil {
		t.Fatal(err)
	}
	assertMoves(t, actor, loc(100), loc(200), loc(0))
}

func TestWalkerArrivedSchedulesDelayAndTickReleases(t *testing.T) {
	routes := route.WalkerRoutes{
		"patrol": {
			"guard": {
				walkerNode(0, withDelay(2000), withNPCString(7), withSocial(9)),
				walkerNode(100),
			},
		},
	}
	now := time.Unix(100, 0)
	actor := &walkerActorStub{id: 1, pos: loc(0)}
	w := newTestWalker(t, routes, &walkerPathStub{canMove: true}, func() time.Time { return now })

	if err := w.StartRoute(actor, "patrol", "guard"); err != nil {
		t.Fatal(err)
	}
	if err := w.Arrived(actor); err != nil {
		t.Fatal(err)
	}
	assertMoves(t, actor, loc(0))
	if got, want := actor.says, []int{7}; !equalInts(got, want) {
		t.Fatalf("says = %v, want %v", got, want)
	}
	if got, want := actor.socials, []int{9}; !equalInts(got, want) {
		t.Fatalf("socials = %v, want %v", got, want)
	}

	now = now.Add(1999 * time.Millisecond)
	if errs := w.Tick(); len(errs) != 0 {
		t.Fatalf("Tick() errors = %v", errs)
	}
	assertMoves(t, actor, loc(0))

	now = now.Add(time.Millisecond)
	actor.moving = true
	if errs := w.Tick(); len(errs) != 0 {
		t.Fatalf("Tick() errors = %v", errs)
	}
	assertMoves(t, actor, loc(0))

	actor.moving = false
	if errs := w.Tick(); len(errs) != 0 {
		t.Fatalf("Tick() errors = %v", errs)
	}
	assertMoves(t, actor, loc(0), loc(100))

	if errs := w.Tick(); len(errs) != 0 {
		t.Fatalf("Tick() errors = %v", errs)
	}
	assertMoves(t, actor, loc(0), loc(100))
}

func TestWalkerReversesWhenNoPathFromRouteEnd(t *testing.T) {
	routes := route.WalkerRoutes{
		"patrol": {
			"guard": {walkerNode(0), walkerNode(100), walkerNode(200)},
		},
	}
	path := &walkerPathStub{canMove: true}
	actor := &walkerActorStub{id: 1, pos: loc(100)}
	w := newTestWalker(t, routes, path, nil)

	if err := w.StartRoute(actor, "patrol", "guard"); err != nil {
		t.Fatal(err)
	}
	actor.pos = loc(100)
	if err := w.Arrived(actor); err != nil {
		t.Fatal(err)
	}
	assertMoves(t, actor, loc(100), loc(200))

	actor.pos = loc(200)
	path.canMove = false
	path.hasPath = false
	if err := w.Arrived(actor); err != nil {
		t.Fatal(err)
	}
	assertMoves(t, actor, loc(100), loc(200), loc(100))
	if actor.geoFails != 1 {
		t.Fatalf("geoFails = %d, want 1", actor.geoFails)
	}

	actor.pos = loc(100)
	path.canMove = true
	if err := w.Arrived(actor); err != nil {
		t.Fatal(err)
	}
	assertMoves(t, actor, loc(100), loc(200), loc(100), loc(0))
}

func TestWalkerTeleportsAfterRepeatedPathFailures(t *testing.T) {
	routes := route.WalkerRoutes{
		"patrol": {
			"guard": {walkerNode(0), walkerNode(100)},
		},
	}
	actor := &walkerActorStub{id: 1, pos: loc(0), geoFails: walkerGeoFailLimit}
	w := newTestWalker(t, routes, &walkerPathStub{canMove: true}, nil)

	if err := w.StartRoute(actor, "patrol", "guard"); err != nil {
		t.Fatal(err)
	}
	if err := w.MoveToNextPoint(actor); err != nil {
		t.Fatal(err)
	}

	assertLocations(t, "teleports", actor.teleports, loc(0))
	if actor.geoFails != 0 {
		t.Fatalf("geoFails = %d, want 0", actor.geoFails)
	}
	if actor.resetGeoFails != 1 {
		t.Fatalf("resetGeoFails = %d, want 1", actor.resetGeoFails)
	}
	assertMoves(t, actor, loc(0), loc(100))
}

func TestWalkerRouteValidation(t *testing.T) {
	w := newTestWalker(t, route.WalkerRoutes{"patrol": {"guard": nil}}, &walkerPathStub{canMove: true}, nil)
	actor := &walkerActorStub{id: 1}

	tests := []struct {
		name  string
		route string
		npc   string
	}{
		{name: "missing route", route: "missing", npc: "guard"},
		{name: "missing npc", route: "patrol", npc: "missing"},
		{name: "empty nodes", route: "patrol", npc: "guard"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := w.StartRoute(actor, test.route, test.npc); err == nil {
				t.Fatal("StartRoute() error = nil")
			}
		})
	}
}

func TestNewWalkerRejectsNilPath(t *testing.T) {
	if _, err := NewWalker(nil, nil, nil); err == nil {
		t.Fatal("NewWalker() error = nil")
	}
}

func TestWalkerMoveErrorsAreReturned(t *testing.T) {
	wantErr := errors.New("move failed")
	routes := route.WalkerRoutes{"patrol": {"guard": {walkerNode(0)}}}
	actor := &walkerActorStub{id: 1, moveErr: wantErr}
	w := newTestWalker(t, routes, &walkerPathStub{canMove: true}, nil)

	if err := w.StartRoute(actor, "patrol", "guard"); !errors.Is(err, wantErr) {
		t.Fatalf("StartRoute() error = %v, want %v", err, wantErr)
	}
}

func TestWalkerTickRetriesRejectedMove(t *testing.T) {
	wantErr := errors.New("move failed")
	routes := route.WalkerRoutes{
		"patrol": {
			"guard": {
				walkerNode(0, withDelay(1000)),
				walkerNode(100),
			},
		},
	}
	now := time.Unix(100, 0)
	actor := &walkerActorStub{id: 1, pos: loc(0)}
	w := newTestWalker(t, routes, &walkerPathStub{canMove: true}, func() time.Time { return now })

	if err := w.StartRoute(actor, "patrol", "guard"); err != nil {
		t.Fatal(err)
	}
	if err := w.Arrived(actor); err != nil {
		t.Fatal(err)
	}

	actor.moveErr = wantErr
	now = now.Add(time.Second)
	errs := w.Tick()
	if len(errs) != 1 || !errors.Is(errs[0], wantErr) {
		t.Fatalf("Tick() errors = %v, want %v", errs, wantErr)
	}
	assertMoves(t, actor, loc(0))

	actor.moveErr = nil
	now = now.Add(WalkerTick)
	if errs := w.Tick(); len(errs) != 0 {
		t.Fatalf("Tick() errors = %v", errs)
	}
	assertMoves(t, actor, loc(0), loc(100))
}

// allocFreeWalkerActor is a zero-allocation WalkerActor stub for allocation-
// ceiling tests: walkerActorStub's move/teleport/say/social slices grow and
// occasionally reallocate, which would add noise to a per-call measurement.
type allocFreeWalkerActor struct {
	id        int32
	pos       location.Location
	moveCount int
}

func (a *allocFreeWalkerActor) ObjectID() int32              { return a.id }
func (a *allocFreeWalkerActor) Position() location.Location  { return a.pos }
func (a *allocFreeWalkerActor) Moving() bool                 { return false }
func (a *allocFreeWalkerActor) TeleportTo(location.Location) {}
func (a *allocFreeWalkerActor) GeoPathFailCount() int        { return 0 }
func (a *allocFreeWalkerActor) ResetGeoPathFailCount()       {}
func (a *allocFreeWalkerActor) AddGeoPathFailCount()         {}
func (a *allocFreeWalkerActor) SayNPCString(id int)          {}
func (a *allocFreeWalkerActor) SocialAction(id int)          {}

func (a *allocFreeWalkerActor) MoveToLocation(target location.Location) (move.Event, error) {
	a.moveCount++
	a.pos = target
	return move.Event{Origin: a.pos, Destination: target}, nil
}

// TestWalkerTickAllocs locks in Walker.Tick's zero-steady-state allocation
// property (#421, #425): the no-release path (nothing due) must stay
// allocation-free as more walkers/AI call sites are wired, and the release
// path's ceiling is 0 too, since a successful release only touches value
// types and interface calls into an allocation-free actor/path — the
// documented non-zero case is the error path (fmt.Errorf on a rejected
// move), covered separately by TestWalkerTickRetriesRejectedMove.
func TestWalkerTickAllocs(t *testing.T) {
	routes := route.WalkerRoutes{
		"patrol": {"guard": {walkerNode(0), walkerNode(100)}},
	}
	path := &walkerPathStub{canMove: true}
	now := time.Unix(100, 0)
	past := now.Add(-time.Second)

	t.Run("no-release path", func(t *testing.T) {
		actor := &allocFreeWalkerActor{id: 1, pos: loc(0)}
		w := newTestWalker(t, routes, path, func() time.Time { return now })
		if err := w.StartRoute(actor, "patrol", "guard"); err != nil {
			t.Fatal(err)
		}
		w.entries[actor.id].wakeTime = time.Time{}

		allocs := testing.AllocsPerRun(1000, func() {
			if errs := w.Tick(); len(errs) != 0 {
				t.Fatalf("Tick() errors = %v, want none", errs)
			}
		})
		if allocs != 0 {
			t.Fatalf("Tick() no-release path allocs/run = %v, want 0", allocs)
		}
	})

	t.Run("release path", func(t *testing.T) {
		actor := &allocFreeWalkerActor{id: 1, pos: loc(0)}
		w := newTestWalker(t, routes, path, func() time.Time { return now })
		if err := w.StartRoute(actor, "patrol", "guard"); err != nil {
			t.Fatal(err)
		}

		allocs := testing.AllocsPerRun(1000, func() {
			w.entries[actor.id].wakeTime = past
			if errs := w.Tick(); len(errs) != 0 {
				t.Fatalf("Tick() errors = %v, want none", errs)
			}
		})
		if allocs != 0 {
			t.Fatalf("Tick() release path allocs/run = %v, want 0", allocs)
		}
		if actor.moveCount == 0 {
			t.Fatal("Tick() release path never called MoveToLocation")
		}
	})
}

func TestGeoPathAdaptsEngineAndFinder(t *testing.T) {
	origin := loc(0)
	target := loc(100)
	geo := &moveGeoStub{canMove: true}
	finder := &pathFinderStub{path: []location.Location{target}, ok: true}
	path := GeoPath{Geo: geo, Finder: finder}

	if !path.CanMove(origin, target) {
		t.Fatal("CanMove() = false, want true")
	}
	if !path.HasPath(origin, target) {
		t.Fatal("HasPath() = false, want true")
	}
	if geo.call != (geoCall{origin: origin, target: target}) {
		t.Fatalf("geo call = %+v, want origin/target", geo.call)
	}
	if finder.origin != origin || finder.target != target {
		t.Fatalf("finder call = %+v -> %+v, want %+v -> %+v", finder.origin, finder.target, origin, target)
	}
}

func TestGeoPathUsesFinderPathCheck(t *testing.T) {
	origin := loc(0)
	target := loc(100)
	finder := &pathCheckerStub{hasPath: true}
	path := GeoPath{Finder: finder}

	if !path.HasPath(origin, target) {
		t.Fatal("HasPath() = false, want true")
	}
	if !finder.checked {
		t.Fatal("HasPath() did not call finder path check")
	}
	if finder.findCalled {
		t.Fatal("HasPath() called Find() and discarded a path")
	}
}

type walkerActorStub struct {
	id            int32
	pos           location.Location
	moving        bool
	moves         []location.Location
	teleports     []location.Location
	says          []int
	socials       []int
	geoFails      int
	resetGeoFails int
	moveErr       error
}

func (a *walkerActorStub) ObjectID() int32 { return a.id }

func (a *walkerActorStub) Position() location.Location { return a.pos }

func (a *walkerActorStub) Moving() bool { return a.moving }

func (a *walkerActorStub) MoveToLocation(target location.Location) (move.Event, error) {
	if a.moveErr != nil {
		return move.Event{}, a.moveErr
	}
	a.moves = append(a.moves, target)
	return move.Event{Origin: a.pos, Destination: target}, nil
}

func (a *walkerActorStub) TeleportTo(target location.Location) {
	a.teleports = append(a.teleports, target)
	a.pos = target
}

func (a *walkerActorStub) GeoPathFailCount() int { return a.geoFails }

func (a *walkerActorStub) ResetGeoPathFailCount() {
	a.geoFails = 0
	a.resetGeoFails++
}

func (a *walkerActorStub) AddGeoPathFailCount() { a.geoFails++ }

func (a *walkerActorStub) SayNPCString(id int) { a.says = append(a.says, id) }

func (a *walkerActorStub) SocialAction(id int) { a.socials = append(a.socials, id) }

type walkerPathStub struct {
	canMove bool
	hasPath bool
}

func (p *walkerPathStub) CanMove(location.Location, location.Location) bool { return p.canMove }

func (p *walkerPathStub) HasPath(location.Location, location.Location) bool { return p.hasPath }

type geoCall struct {
	origin location.Location
	target location.Location
}

type moveGeoStub struct {
	canMove bool
	call    geoCall
}

func (g *moveGeoStub) CanMove(ox, oy, oz, tx, ty, tz int) bool {
	g.call = geoCall{origin: location.Location{X: ox, Y: oy, Z: oz}, target: location.Location{X: tx, Y: ty, Z: tz}}
	return g.canMove
}

type pathFinderStub struct {
	path   []location.Location
	ok     bool
	origin location.Location
	target location.Location
}

func (f *pathFinderStub) Find(origin, target location.Location) ([]location.Location, int, bool) {
	f.origin = origin
	f.target = target
	return f.path, len(f.path), f.ok
}

type pathCheckerStub struct {
	hasPath    bool
	checked    bool
	findCalled bool
	origin     location.Location
	target     location.Location
}

func (f *pathCheckerStub) Find(origin, target location.Location) ([]location.Location, int, bool) {
	f.findCalled = true
	return []location.Location{target}, 1, true
}

func (f *pathCheckerStub) HasPath(origin, target location.Location) bool {
	f.checked = true
	f.origin = origin
	f.target = target
	return f.hasPath
}

type walkerNodeOption func(*route.WalkerLocation)

func withDelay(delayMillis int) walkerNodeOption {
	return func(node *route.WalkerLocation) { node.DelayMillis = delayMillis }
}

func withNPCString(id int) walkerNodeOption {
	return func(node *route.WalkerLocation) { node.NPCStringID = id }
}

func withSocial(id int) walkerNodeOption {
	return func(node *route.WalkerLocation) { node.SocialID = id }
}

func walkerNode(x int, opts ...walkerNodeOption) route.WalkerLocation {
	node := route.WalkerLocation{Location: loc(x)}
	for _, opt := range opts {
		opt(&node)
	}
	return node
}

func loc(x int) location.Location {
	return location.Location{X: x, Y: 10, Z: 20}
}

func newTestWalker(t *testing.T, routes route.WalkerRoutes, path WalkerPath, now func() time.Time) *Walker {
	t.Helper()
	w, err := NewWalker(routes, path, now)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func assertMoves(t *testing.T, actor *walkerActorStub, want ...location.Location) {
	t.Helper()
	assertLocations(t, "moves", actor.moves, want...)
}

func assertLocations(t *testing.T, name string, got []location.Location, want ...location.Location) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %+v, want %+v", name, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s = %+v, want %+v", name, got, want)
		}
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
