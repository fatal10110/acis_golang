package move

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// ---- from scatter_test.go ----

// reachableGeo is flat open terrain at a fixed ground height: every scatter
// reaches its target.
type reachableGeo struct{ staticGeo }

func (reachableGeo) ValidLocation(_, _, _, tx, ty, tz int) location.Location {
	return location.Location{X: tx, Y: ty, Z: tz}
}

func TestRandomNearbyLocationSnapsHeightAndStaysWithinOffset(t *testing.T) {
	geo := reachableGeo{staticGeo{canMove: true, height: 42}}
	target := location.Location{X: 1000, Y: 1000, Z: 0}

	for range 200 {
		got := RandomNearbyLocation(geo, target, 20, nil)
		if got.Z != 42 {
			t.Fatalf("Z = %d, want snapped height 42", got.Z)
		}
		if dx := got.X - target.X; dx < -20 || dx > 20 {
			t.Fatalf("X = %d, want within 20 of %d", got.X, target.X)
		}
		if dy := got.Y - target.Y; dy < -20 || dy > 20 {
			t.Fatalf("Y = %d, want within 20 of %d", got.Y, target.Y)
		}
	}
}

// TestRandomNearbyLocationClipsScatterToLastReachablePoint pins the scatter
// validation of Creature.teleportTo (Creature.java:399-407): the scattered
// point goes through GeoEngine.getValidLocation(x, y, z, nx, ny, z) — walked
// at the destination's own height — and the teleport lands on the last
// reachable x/y it returns, not back on the unscattered target. Ground
// height is then probed at the clipped point.
func TestRandomNearbyLocationClipsScatterToLastReachablePoint(t *testing.T) {
	clipped := location.Location{X: 1007, Y: 995, Z: -300}
	geo := &recordingGeo{canMove: false, height: 42, validLocation: clipped}
	target := location.Location{X: 1000, Y: 1000, Z: -120}

	got := RandomNearbyLocation(geo, target, 20, nil)

	if want := (location.Location{X: 1007, Y: 995, Z: 42}); got != want {
		t.Fatalf("RandomNearbyLocation() = %+v, want clipped x/y with snapped height %+v", got, want)
	}
	if len(geo.moveCalls) != 0 {
		t.Fatalf("CanMove calls = %d, want none (the scatter is clipped, not gated)", len(geo.moveCalls))
	}
	if len(geo.validLocationCalls) != 1 {
		t.Fatalf("ValidLocation calls = %d, want 1", len(geo.validLocationCalls))
	}
	call := geo.validLocationCalls[0]
	if call.origin != target {
		t.Fatalf("ValidLocation origin = %+v, want the unscattered target %+v", call.origin, target)
	}
	if call.target.Z != target.Z {
		t.Fatalf("ValidLocation target Z = %d, want the target's own height %d", call.target.Z, target.Z)
	}
	if dx, dy := call.target.X-target.X, call.target.Y-target.Y; dx < -20 || dx > 20 || dy < -20 || dy > 20 {
		t.Fatalf("scattered point %+v, want within 20 of %+v", call.target, target)
	}
	if want := []location.Location{{X: 1007, Y: 995, Z: -120}}; len(geo.heightCalls) != 1 || geo.heightCalls[0] != want[0] {
		t.Fatalf("Height calls = %+v, want %+v", geo.heightCalls, want)
	}
}

func TestRandomNearbyLocationNoProgressKeepsTarget(t *testing.T) {
	geo := &recordingGeo{height: 7}
	target := location.Location{X: 1000, Y: 1000, Z: 0}

	got := RandomNearbyLocation(geo, target, 20, nil)

	if want := (location.Location{X: 1000, Y: 1000, Z: 7}); got != want {
		t.Fatalf("RandomNearbyLocation() = %+v, want unscattered target snapped %+v", got, want)
	}
}

// TestRandomNearbyLocationKeepsHeightWhenAsked pins the height rule of
// Creature.teleportTo (Creature.java:409-411): Z snaps to ground only when
// the creature is not flying and the destination — the scattered x/y at the
// requested z — is not inside a water zone.
func TestRandomNearbyLocationKeepsHeightWhenAsked(t *testing.T) {
	clipped := location.Location{X: 1010, Y: 990, Z: -900}
	geo := &recordingGeo{height: -900, validLocation: clipped}
	target := location.Location{X: 1000, Y: 1000, Z: -120}
	var asked []location.Location

	got := RandomNearbyLocation(geo, target, 20, func(at location.Location) bool {
		asked = append(asked, at)
		return true
	})

	if want := (location.Location{X: 1010, Y: 990, Z: -120}); got != want {
		t.Fatalf("RandomNearbyLocation() = %+v, want clipped x/y at the requested height %+v", got, want)
	}
	if want := (location.Location{X: 1010, Y: 990, Z: -120}); len(asked) != 1 || asked[0] != want {
		t.Fatalf("keepHeight asked %+v, want once at %+v", asked, want)
	}
	if len(geo.heightCalls) != 0 {
		t.Fatalf("Height calls = %+v, want none when the height is kept", geo.heightCalls)
	}
}

func TestRandomNearbyLocationSnapsWhenKeepHeightDeclines(t *testing.T) {
	geo := &recordingGeo{height: -900}
	target := location.Location{X: 1000, Y: 1000, Z: -120}

	got := RandomNearbyLocation(geo, target, 0, func(location.Location) bool { return false })

	if want := (location.Location{X: 1000, Y: 1000, Z: -900}); got != want {
		t.Fatalf("RandomNearbyLocation() = %+v, want snapped %+v", got, want)
	}
}

func TestRandomNearbyLocationSkipsScatterForNonPositiveOffset(t *testing.T) {
	geo := &recordingGeo{height: 9}
	target := location.Location{X: 1000, Y: 1000, Z: 0}

	got := RandomNearbyLocation(geo, target, 0, nil)

	if want := (location.Location{X: 1000, Y: 1000, Z: 9}); got != want {
		t.Fatalf("RandomNearbyLocation() = %+v, want unscattered target snapped %+v (offset <= 0)", got, want)
	}
	if len(geo.validLocationCalls) != 0 {
		t.Fatalf("ValidLocation calls = %d, want none for offset <= 0", len(geo.validLocationCalls))
	}
}

func TestRandomNearbyLocationNilGeoReturnsTargetUnchanged(t *testing.T) {
	target := location.Location{X: 1000, Y: 1000, Z: 5}

	got := RandomNearbyLocation(nil, target, 20, nil)

	if got != target {
		t.Fatalf("RandomNearbyLocation(nil, ...) = %+v, want unchanged %+v", got, target)
	}
}
