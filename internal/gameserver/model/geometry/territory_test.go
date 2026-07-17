package geometry

import "testing"

func TestNewTerritoryRejectsInvalidInput(t *testing.T) {
	rect := NewRectangle(0, 10, 0, 10)
	if _, err := NewTerritory(0, 10); err == nil {
		t.Error("NewTerritory with no shapes succeeded, want error")
	}
	if _, err := NewTerritory(10, 0, rect); err == nil {
		t.Error("NewTerritory with minZ > maxZ succeeded, want error")
	}
}

func TestTerritoryContainsUnionAndZRange(t *testing.T) {
	a := NewRectangle(0, 10, 0, 10)
	b := NewRectangle(100, 110, 100, 110)
	tr, err := NewTerritory(-50, 50, a, b)
	if err != nil {
		t.Fatalf("NewTerritory: %v", err)
	}
	cases := []struct {
		x, y, z int
		want    bool
	}{
		{5, 5, 0, true},     // inside shape a, mid z-range
		{105, 105, 0, true}, // inside shape b (union)
		{5, 5, -50, true},   // z at the low bound, inclusive
		{5, 5, 50, true},    // z at the high bound, inclusive
		{5, 5, -51, false},  // z just below the range
		{5, 5, 51, false},   // z just above the range
		{50, 50, 0, false},  // between the two shapes, in neither
	}
	for _, c := range cases {
		if got := tr.Contains(c.x, c.y, c.z); got != c.want {
			t.Errorf("Contains(%d,%d,%d) = %v, want %v", c.x, c.y, c.z, got, c.want)
		}
	}
}

func TestTerritoryLowHighZ(t *testing.T) {
	tr, err := NewTerritory(-10, 20, NewRectangle(0, 1, 0, 1))
	if err != nil {
		t.Fatalf("NewTerritory: %v", err)
	}
	if tr.LowZ() != -10 || tr.HighZ() != 20 {
		t.Errorf("LowZ/HighZ = %d/%d, want -10/20", tr.LowZ(), tr.HighZ())
	}
}

func TestTerritoryArea(t *testing.T) {
	a := NewRectangle(0, 10, 0, 10) // area 100
	b := NewRectangle(0, 5, 0, 5)   // area 25, overlaps a but is not deduplicated
	tr, err := NewTerritory(0, 10, a, b)
	if err != nil {
		t.Fatalf("NewTerritory: %v", err)
	}
	if got := tr.Area(); got != 125 {
		t.Errorf("Area() = %v, want 125 (sum, not union)", got)
	}
}

func TestTerritoryIntersects(t *testing.T) {
	overlapping, err := NewTerritory(0, 10, NewRectangle(0, 10, 0, 10))
	if err != nil {
		t.Fatalf("NewTerritory: %v", err)
	}
	sameXYDisjointZ, err := NewTerritory(20, 30, NewRectangle(0, 10, 0, 10))
	if err != nil {
		t.Fatalf("NewTerritory: %v", err)
	}
	sameZDisjointXY, err := NewTerritory(0, 10, NewRectangle(1000, 1010, 1000, 1010))
	if err != nil {
		t.Fatalf("NewTerritory: %v", err)
	}
	both, err := NewTerritory(5, 15, NewRectangle(5, 15, 5, 15))
	if err != nil {
		t.Fatalf("NewTerritory: %v", err)
	}

	if overlapping.Intersects(sameXYDisjointZ) {
		t.Error("territories with disjoint z ranges reported as intersecting")
	}
	if overlapping.Intersects(sameZDisjointXY) {
		t.Error("territories with disjoint footprints reported as intersecting")
	}
	if !overlapping.Intersects(both) {
		t.Error("territories overlapping in both z range and footprint reported as not intersecting")
	}
}
