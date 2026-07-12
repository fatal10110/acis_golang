package location

import "testing"

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
