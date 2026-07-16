package location

import "testing"

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
