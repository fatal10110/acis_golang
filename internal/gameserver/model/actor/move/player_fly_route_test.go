package move

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// PlayerMove.calculatePath (PlayerMove.java:418-438): a flying player's line
// is open by GeoEngine.canFlyToTarget with a 32-high corridor, a swimming
// one's by the ground canMoveToTarget; a line that is not ends at
// getValidFlyLocation, with no geopath (findPath) for either.
func TestSwimmingOrFlyingPlayerRoute(t *testing.T) {
	target := location.Location{X: 300, Y: 40, Z: 120}
	stop := location.Location{X: 150, Y: 20, Z: 60}
	tests := []struct {
		name      string
		flying    bool
		canMove   bool
		canFly    bool
		want      location.Location
		wantMoves int
		wantFlies int
		wantStops int
	}{
		{name: "swimmer, ground line open", canMove: true, want: target, wantMoves: 1},
		{name: "swimmer, ground line closed", canFly: true, want: stop, wantMoves: 1, wantStops: 1},
		{name: "flier, fly line open", flying: true, canFly: true, want: target, wantFlies: 1},
		{name: "flier, fly line closed", flying: true, canMove: true, want: stop, wantFlies: 1, wantStops: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			geo := &recordingGeo{
				canMove:          tt.canMove,
				canFly:           tt.canFly,
				findPath:         []location.Location{{X: 100}, {X: 200}, target},
				findPathOK:       true,
				validFlyLocation: stop,
			}
			mover, _ := newTestMover(t, geo)
			mover.SetSpeeds(100, 100)
			if tt.flying {
				mover.SetFlying(true)
			} else {
				mover.SetWaterSurface(everywhereWater)
			}
			ev, err := mover.MoveToLocation(target)
			if err != nil {
				t.Fatal(err)
			}
			if ev.Destination != tt.want {
				t.Fatalf("destination = %+v, want %+v", ev.Destination, tt.want)
			}
			if len(mover.waypoints) != 0 {
				t.Fatalf("queued waypoints %+v, want none", mover.waypoints)
			}
			if len(geo.findPathCalls) != 0 || len(geo.validLocationCalls) != 0 {
				t.Fatalf("ground route queries: %d FindPath, %d ValidLocation; want none", len(geo.findPathCalls), len(geo.validLocationCalls))
			}
			if len(geo.moveCalls) != tt.wantMoves || len(geo.flyCalls) != tt.wantFlies || len(geo.validFlyCalls) != tt.wantStops {
				t.Fatalf("queries: %d CanMove, %d CanFly, %d ValidFlyLocation; want %d, %d, %d",
					len(geo.moveCalls), len(geo.flyCalls), len(geo.validFlyCalls), tt.wantMoves, tt.wantFlies, tt.wantStops)
			}
			for _, call := range append(geo.flyCalls, geo.validFlyCalls...) {
				want := flyCall{target: target, height: 32}
				if call != want {
					t.Fatalf("fly query %+v, want %+v", call, want)
				}
			}
		})
	}
}

// CreatureMove.calculatePath, which every mover other than a player takes,
// routes a swimmer or flier on the ground: findPath, then getValidLocation.
func TestSwimmingOrFlyingNonPlayerRoutesOnGround(t *testing.T) {
	for _, flying := range []bool{true, false} {
		geo := &recordingGeo{
			canFly:     true,
			findPath:   []location.Location{{X: 100}, {X: 300, Y: 40, Z: 120}},
			findPathOK: true,
		}
		mover, _ := newTestMover(t, geo)
		if flying {
			mover.SetFlying(true)
		} else {
			mover.SetWaterSurface(everywhereWater)
		}
		ev, err := mover.MoveToLocation(location.Location{X: 300, Y: 40, Z: 120})
		if err != nil {
			t.Fatal(err)
		}
		if ev.Destination != (location.Location{X: 100}) || len(mover.waypoints) != 1 {
			t.Fatalf("flying %v: route to %+v then %+v, want the geopath", flying, ev.Destination, mover.waypoints)
		}
		if len(geo.flyCalls) != 0 || len(geo.validFlyCalls) != 0 {
			t.Fatalf("flying %v: %d fly queries, want none", flying, len(geo.flyCalls)+len(geo.validFlyCalls))
		}
	}
}

// The water surface caps a swimming creature's height
// (CreatureMove.java:317-323) but never a swimming player's: a player is
// capped only while flying (PlayerMove.java:224-257), and swimming outranks
// flying in a water zone (CreatureMove.getMoveType), so a flier there swims
// uncapped too.
func TestWaterSurfaceCapsOnlyNonPlayerSwimmers(t *testing.T) {
	shallowSurface := func(location.Location) (int, bool) { return 50, true }
	tests := []struct {
		name   string
		player bool
		flying bool
		want   int
	}{
		{name: "creature", want: 50},
		{name: "player", player: true, want: 200},
		{name: "flying player", player: true, flying: true, want: 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mover, _ := newTestMover(t, staticGeo{canMove: true})
			mover.SetWaterSurface(shallowSurface)
			mover.SetFlying(tt.flying)
			if tt.player {
				mover.SetSpeeds(100, 100)
			}
			if got := mover.MoveType(); got != MoveSwim {
				t.Fatalf("MoveType() = %d, want MoveSwim", got)
			}
			if _, err := mover.MoveToLocation(location.Location{X: 30, Z: 200}); err != nil {
				t.Fatal(err)
			}
			// The 202-unit climb takes 21 updates at 10 units each; a capped
			// swimmer never closes it and keeps treading the surface.
			for range 40 {
				mover.UpdatePosition(PositionUpdateInterval)
			}
			if got := mover.Position(); got.Z != tt.want {
				t.Fatalf("swimmer at %+v, want height %d", got, tt.want)
			}
		})
	}
}
