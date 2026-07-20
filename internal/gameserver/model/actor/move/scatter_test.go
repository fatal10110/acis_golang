package move

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func TestRandomNearbyLocationSnapsHeightAndStaysWithinOffset(t *testing.T) {
	geo := staticGeo{canMove: true, height: 42}
	target := location.Location{X: 1000, Y: 1000, Z: 0}

	got := RandomNearbyLocation(geo, target, 20)

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

func TestRandomNearbyLocationKeepsTargetWhenScatterBlocked(t *testing.T) {
	geo := staticGeo{canMove: false, height: 7}
	target := location.Location{X: 1000, Y: 1000, Z: 0}

	got := RandomNearbyLocation(geo, target, 20)

	if got.X != target.X || got.Y != target.Y {
		t.Fatalf("X,Y = %d,%d, want unchanged target %d,%d (scatter blocked)", got.X, got.Y, target.X, target.Y)
	}
	if got.Z != 7 {
		t.Fatalf("Z = %d, want snapped height 7", got.Z)
	}
}

func TestRandomNearbyLocationSkipsScatterForNonPositiveOffset(t *testing.T) {
	geo := staticGeo{canMove: true, height: 9}
	target := location.Location{X: 1000, Y: 1000, Z: 0}

	got := RandomNearbyLocation(geo, target, 0)

	if got.X != target.X || got.Y != target.Y {
		t.Fatalf("X,Y = %d,%d, want unchanged target %d,%d (offset <= 0)", got.X, got.Y, target.X, target.Y)
	}
}

func TestRandomNearbyLocationNilGeoReturnsTargetUnchanged(t *testing.T) {
	target := location.Location{X: 1000, Y: 1000, Z: 5}

	got := RandomNearbyLocation(nil, target, 20)

	if got != target {
		t.Fatalf("RandomNearbyLocation(nil, ...) = %+v, want unchanged %+v", got, target)
	}
}
