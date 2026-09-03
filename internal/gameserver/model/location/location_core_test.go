package location

import (
	"testing"
)

// ---- from heading_test.go ----
func TestOrientedLocationFrontAndBehind(t *testing.T) {
	origin := OrientedLocation{Location: Location{X: 0, Y: 0}, Heading: 0}

	if !origin.IsInFrontOf(Location{X: 80, Y: 0}) {
		t.Fatal("IsInFrontOf() = false for point directly ahead")
	}
	if origin.IsInFrontOf(Location{X: -80, Y: 0}) {
		t.Fatal("IsInFrontOf() = true for point behind")
	}
	if !origin.IsBehind(Location{X: -80, Y: 0}) {
		t.Fatal("IsBehind() = false for point directly behind")
	}
	if origin.IsBehind(Location{X: 80, Y: 0}) {
		t.Fatal("IsBehind() = true for point ahead")
	}
}

func TestOrientedLocationFrontAndBehindWrapAround(t *testing.T) {
	north := OrientedLocation{Location: Location{X: 0, Y: 0}, Heading: 16384}

	if !north.IsInFrontOf(Location{X: 0, Y: 80}) {
		t.Fatal("IsInFrontOf() = false for north-facing point ahead")
	}
	if !north.IsBehind(Location{X: 0, Y: -80}) {
		t.Fatal("IsBehind() = false for north-facing point behind")
	}
}

// ---- from location_test.go ----
func TestLocationDistance3D(t *testing.T) {
	got := (Location{X: 10, Y: 20, Z: 30}).Distance3D(Location{X: 13, Y: 24, Z: 42})
	if got != 13 {
		t.Fatalf("Distance3D() = %v, want 13", got)
	}
}

func TestLocationIn3DRange(t *testing.T) {
	origin := Location{X: 10, Y: 20, Z: 30}
	other := Location{X: 13, Y: 24, Z: 42}

	if !origin.In3DRange(other, 13) {
		t.Fatal("In3DRange() = false at exact radius")
	}
	if origin.In3DRange(other, 12) {
		t.Fatal("In3DRange() = true outside radius")
	}
	if origin.In3DRange(other, -13) {
		t.Fatal("In3DRange() = true for negative radius")
	}
}

func TestIn3DRange(t *testing.T) {
	if !In3DRange(10, 20, 30, 13, 24, 42, 13) {
		t.Fatal("In3DRange() = false at exact radius")
	}
	if In3DRange(10, 20, 30, 13, 24, 42, 12) {
		t.Fatal("In3DRange() = true outside radius")
	}
	if In3DRange(10, 20, 30, 13, 24, 42, -13) {
		t.Fatal("In3DRange() = true for negative radius")
	}
}

func TestIn2DRadius(t *testing.T) {
	origin := Location{X: 0, Y: 0, Z: 0}
	// 3-4-5 triangle: ground distance 5.
	onBoundary := Location{X: 3, Y: 4, Z: 1000}

	if origin.In2DRadius(onBoundary, 5) {
		t.Fatal("In2DRadius() = true at exact radius; want strict exclusion")
	}
	if !origin.In2DRadius(onBoundary, 6) {
		t.Fatal("In2DRadius() = false one unit inside radius")
	}
	if origin.In2DRadius(Location{X: 6, Y: 0, Z: 0}, 5) {
		t.Fatal("In2DRadius() = true one unit outside radius")
	}
	if !origin.In2DRadius(Location{X: 0, Y: 0, Z: 1000}, 1) {
		t.Fatal("In2DRadius() = false for altitude-only offset")
	}
	if In2DRadius(0, 0, 3, 4, 5) {
		t.Fatal("In2DRadius() package helper = true at exact radius")
	}
	if !In2DRadius(0, 0, 3, 4, 6) {
		t.Fatal("In2DRadius() package helper = false inside radius")
	}
}

func TestAddRandomOffsetBetweenRejectsInvalidRange(t *testing.T) {
	origin := Location{X: 10, Y: 20, Z: 30}
	cases := []struct{ min, max int }{
		{-1, 10},
		{10, -1},
		{10, 9},
	}
	for _, c := range cases {
		if got := origin.AddRandomOffsetBetween(c.min, c.max); got != origin {
			t.Errorf("AddRandomOffsetBetween(%d, %d) = %+v, want unchanged origin", c.min, c.max, got)
		}
	}
}
