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

func TestIn3DRadius(t *testing.T) {
	origin := Location{X: 0, Y: 0, Z: 0}
	// 3-4-12 triangle: straight-line distance 13.
	onBoundary := Location{X: 3, Y: 4, Z: 12}

	if origin.In3DRadius(onBoundary, 13) {
		t.Fatal("In3DRadius() = true at exact radius; want strict exclusion")
	}
	if !origin.In3DRadius(onBoundary, 14) {
		t.Fatal("In3DRadius() = false one unit inside radius")
	}
	if origin.In3DRadius(Location{X: 14, Y: 0, Z: 0}, 13) {
		t.Fatal("In3DRadius() = true one unit outside radius")
	}
}

func TestIn3DRadiusPackageHelper(t *testing.T) {
	// 3-4-12 triangle offset from a non-origin point: distance 13.
	if In3DRadius(10, 20, 30, 13, 24, 42, 13) {
		t.Fatal("In3DRadius() package helper = true at exact radius")
	}
	if !In3DRadius(10, 20, 30, 13, 24, 42, 14) {
		t.Fatal("In3DRadius() package helper = false inside radius")
	}
	if In3DRadius(10, 20, 30, 10, 20, 30, 0) {
		t.Fatal("In3DRadius() package helper = true for a zero radius")
	}
	if In3DRadius(10, 20, 30, 10, 20, 30, -1) {
		t.Fatal("In3DRadius() package helper = true for negative radius")
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

// TestLocationFleeFrom pins the flee point against vectors computed with IEEE
// double arithmetic and truncating int casts, the reference arithmetic:
// the offset splits distance between the axes by the |dx/dy| ratio, and a
// source on the same Y line (or the point itself) produces no offset.
func TestLocationFleeFrom(t *testing.T) {
	cases := []struct {
		from       Location
		srcX, srcY int
		want       Location
	}{
		{Location{X: 0, Y: 0, Z: 7}, 100, 100, Location{X: -250, Y: -250, Z: 7}},
		{Location{X: 1000, Y: 2000, Z: -30}, 1300, 2100, Location{X: 625, Y: 1875, Z: -30}},
		{Location{X: 0, Y: 0}, -7, 3, Location{X: 350, Y: -150}},
		{Location{X: 0, Y: 0}, 0, 1, Location{X: 0, Y: -500}},
		{Location{X: -80000, Y: 150000}, -80400, 150300, Location{X: -79715, Y: 149786}},
		{Location{X: 10, Y: 10}, 11, -1000, Location{X: 10, Y: 509}},
		{Location{X: 0, Y: 0}, 100, 0, Location{X: 0, Y: 0}},
		{Location{X: 5, Y: 5}, 5, 5, Location{X: 5, Y: 5}},
	}
	for _, tc := range cases {
		if got := tc.from.FleeFrom(tc.srcX, tc.srcY, 500); got != tc.want {
			t.Errorf("%+v.FleeFrom(%d, %d, 500) = %+v, want %+v", tc.from, tc.srcX, tc.srcY, got, tc.want)
		}
	}
}

// TestEquidistantPoint pins Circle.getEquidistantPoints(12, z) for a radius
// 70 circle (SummonMove.avoidAttack). Oracle: the reference method run on a
// JVM, printing each point's offset from the center; note the truncated
// 34s where the sine or cosine of 30-degree steps rounds just under 0.5.
func TestEquidistantPoint(t *testing.T) {
	want := [12][2]int{
		{70, 0},
		{60, 34},
		{35, 60},
		{0, 70},
		{-34, 60},
		{-60, 35},
		{-70, 0},
		{-60, -34},
		{-35, -60},
		{0, -70},
		{34, -60},
		{60, -35},
	}
	for i, w := range want {
		got := EquidistantPoint(1000, -2000, 55, 70, 12, i)
		if got != (Location{X: 1000 + w[0], Y: -2000 + w[1], Z: 55}) {
			t.Errorf("point %d = %+v, want offset %v at z 55", i, got, w)
		}
	}
}
