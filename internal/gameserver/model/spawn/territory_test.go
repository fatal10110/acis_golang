package spawn

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
)

func testTerritorySet(name string, minZ, maxZ int) *commons.StatSet {
	set := commons.NewStatSetWithCapacity(3)
	set.Set("name", name)
	set.Set("minZ", minZ)
	set.Set("maxZ", maxZ)
	return set
}

func TestNewTerritoryRejectsTooFewNodes(t *testing.T) {
	nodes := []Node{{X: 0, Y: 0}, {X: 10, Y: 0}}
	if _, err := NewTerritory(testTerritorySet("t", 0, 100), nodes); err == nil {
		t.Error("NewTerritory with 2 nodes succeeded, want error")
	}
}

func TestNewTerritoryRejectsInvertedZRange(t *testing.T) {
	nodes := []Node{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 0, Y: 10}}
	if _, err := NewTerritory(testTerritorySet("t", 100, 0), nodes); err == nil {
		t.Error("NewTerritory with maxZ < minZ succeeded, want error")
	}
}

func TestTerritoryGeometryMatchesFields(t *testing.T) {
	nodes := []Node{{X: 0, Y: 0}, {X: 100, Y: 0}, {X: 100, Y: 100}, {X: 0, Y: 100}}
	tr, err := NewTerritory(testTerritorySet("t1", -50, 50), nodes)
	if err != nil {
		t.Fatalf("NewTerritory: %v", err)
	}

	cases := []struct {
		x, y, z int
		want    bool
	}{
		{50, 50, 0, true},    // interior, mid z
		{50, 50, -50, true},  // z at low bound, inclusive
		{50, 50, 50, true},   // z at high bound, inclusive
		{50, 50, -51, false}, // below the declared range
		{50, 50, 51, false},  // above the declared range
		{200, 200, 0, false}, // outside the polygon footprint
	}
	for _, c := range cases {
		if got := tr.Contains(c.x, c.y, c.z); got != c.want {
			t.Errorf("Contains(%d, %d, %d) = %v, want %v", c.x, c.y, c.z, got, c.want)
		}
	}

	if got, want := tr.Area(), 10000.0; got != want {
		t.Errorf("Area() = %v, want %v", got, want)
	}

	other, err := NewTerritory(testTerritorySet("t2", -50, 50), []Node{{X: 50, Y: 50}, {X: 150, Y: 50}, {X: 150, Y: 150}, {X: 50, Y: 150}})
	if err != nil {
		t.Fatalf("NewTerritory: %v", err)
	}
	if !tr.Intersects(other.Territory) {
		t.Error("overlapping territories reported as not intersecting")
	}
}

func TestTerritoryLiteralWithoutGeometryStaysUsable(t *testing.T) {
	// Existing callers (e.g. test fixtures elsewhere) build a Territory as
	// a plain struct literal, leaving the embedded *geometry.Territory
	// nil. The legacy fields must stay directly usable in that case.
	tr := &Territory{Name: "t", MinZ: -100, MaxZ: 100, Nodes: []Node{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 0, Y: 10}}}
	if tr.Name != "t" || tr.MinZ != -100 || tr.MaxZ != 100 || len(tr.Nodes) != 3 {
		t.Error("literal-constructed Territory lost its field values")
	}
}
