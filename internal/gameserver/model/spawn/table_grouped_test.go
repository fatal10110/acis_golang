package spawn

import "testing"

// TestTableGroupsMultiTerritoryMakers pins the merged territories: one
// grouped_NNN per maker naming several, numbered over the makers in list
// order, territory before banned territory, spanning the members' Z.
func TestTableGroupsMultiTerritoryMakers(t *testing.T) {
	a, b, c := newFixtureTerritory("a"), newFixtureTerritory("b"), newFixtureTerritory("c")
	b.MinZ, c.MaxZ = -50, 300
	single := newFixtureMaker("single", a)
	pair := newFixtureMaker("pair", a, b)
	pair.BannedTerritories = []*Territory{b, c}
	banned := newFixtureMaker("banned", c)
	banned.BannedTerritories = []*Territory{a}
	triple := newFixtureMaker("triple", a, b, c)
	table, err := NewTable([]*Territory{a, b, c}, []*Maker{single, pair, banned, triple})
	if err != nil {
		t.Fatalf("NewTable: %v", err)
	}

	if got := table.MakerTerritory(single); got != a {
		t.Fatalf("single territory = %v, want a", got)
	}
	if got := table.MakerBannedTerritory(single); got != nil {
		t.Fatalf("single banned = %v, want none", got)
	}
	if got := table.MakerBannedTerritory(banned); got != a {
		t.Fatalf("banned banned = %v, want a", got)
	}
	for _, tc := range []struct {
		got        *Territory
		name       string
		minZ, maxZ int
		triangles  int
	}{
		{table.MakerTerritory(pair), "grouped_000", -50, 100, 2},
		{table.MakerBannedTerritory(pair), "grouped_001", -50, 300, 2},
		{table.MakerTerritory(triple), "grouped_002", -50, 300, 3},
	} {
		if tc.got == nil || tc.got.Name != tc.name || tc.got.MinZ != tc.minZ || tc.got.MaxZ != tc.maxZ || len(tc.got.Triangles()) != tc.triangles {
			t.Fatalf("merged = %+v, want %s over %d..%d with %d triangles", tc.got, tc.name, tc.minZ, tc.maxZ, tc.triangles)
		}
		if found, ok := table.Territory(tc.name); !ok || found != tc.got {
			t.Fatalf("Territory(%q) = %v, %v; want the merged territory", tc.name, found, ok)
		}
	}
	if got := table.TerritoryCount(); got != 3 {
		t.Fatalf("TerritoryCount = %d, want the 3 declared", got)
	}
}
