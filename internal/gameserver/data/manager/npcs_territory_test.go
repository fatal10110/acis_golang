package manager

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

// constGeoZ.Height always returns the same z regardless of x, y, or the
// caller's average-z seed, so tests can isolate the Z-range-check logic
// from the geodata query itself.
type constGeoZ struct{ z int16 }

func (g constGeoZ) CanMove(int, int, int, int, int, int) bool { return true }
func (g constGeoZ) Height(int, int, int) int16                { return g.z }
func (constGeoZ) FindPath(_, _ location.Location) ([]location.Location, bool) {
	return nil, false
}
func (constGeoZ) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

func TestMergedZRangeIsMinOfMinsMaxOfMaxes(t *testing.T) {
	territories := []*spawn.Territory{
		{Name: "a", MinZ: -100, MaxZ: 50, Nodes: []spawn.Node{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 0, Y: 10}}},
		{Name: "b", MinZ: 20, MaxZ: 300, Nodes: []spawn.Node{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 0, Y: 10}}},
	}

	minZ, maxZ := mergedZRange(territories)
	if minZ != -100 || maxZ != 300 {
		t.Fatalf("mergedZRange() = (%d,%d), want (-100,300)", minZ, maxZ)
	}
}

func TestWeightedTerritoryPickFavorsLargerArea(t *testing.T) {
	big := &spawn.Territory{Name: "big", Nodes: []spawn.Node{{X: 0, Y: 0}, {X: 1000, Y: 0}, {X: 0, Y: 1000}}}    // area 500000
	small := &spawn.Territory{Name: "small", Nodes: []spawn.Node{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 0, Y: 10}}}    // area 50
	territories := []*spawn.Territory{big, small}

	const trials = 5000
	bigCount := 0
	for i := 0; i < trials; i++ {
		if weightedTerritoryPick(territories) == big {
			bigCount++
		}
	}

	// big is ~10000x small's area, so it should dominate selection; a loose
	// 90% floor distinguishes this from the old uniform 50/50 pick without
	// making the test flaky.
	if frac := float64(bigCount) / trials; frac < 0.90 {
		t.Fatalf("weightedTerritoryPick picked big %.3f of the time, want >0.90 (old uniform pick would give ~0.5)", frac)
	}
}

// TestRandomTerritoryPositionUsesMergedZRangeNotSubTerritoryOwnRange
// reproduces PR 552 review finding 1: a multi-territory maker's spawn-time
// Z check must validate against the merged min-of-mins/max-of-maxes Z range
// (SpawnManager.findTerritory, Territory.java's merged _minZ/_maxZ), not
// each sub-territory's own MinZ/MaxZ. Territory B's own range excludes the
// z the geo mock always returns, but the merged range (driven by A) includes
// it, so a fixed spawn-time Z check must still accept points landing in B.
func TestRandomTerritoryPositionUsesMergedZRangeNotSubTerritoryOwnRange(t *testing.T) {
	territoryA := &spawn.Territory{
		Name: "a", MinZ: 500, MaxZ: 500,
		Nodes: []spawn.Node{{X: 0, Y: 0}, {X: 100, Y: 0}, {X: 0, Y: 100}},
	}
	territoryB := &spawn.Territory{
		Name: "b", MinZ: -500, MaxZ: -500,
		Nodes: []spawn.Node{{X: 1000, Y: 0}, {X: 1100, Y: 0}, {X: 1000, Y: 100}},
	}
	maker := &spawn.Maker{Territories: []*spawn.Territory{territoryA, territoryB}}
	geo := constGeoZ{500}

	const trials = 2000
	inB := 0
	for i := 0; i < trials; i++ {
		pos, ok := randomTerritoryPosition(maker, geo)
		if !ok {
			t.Fatalf("randomTerritoryPosition() ok = false, want true")
		}
		if pos.Location.Z != 500 {
			t.Fatalf("randomTerritoryPosition() z = %d, want 500", pos.Location.Z)
		}
		if pos.Location.X >= 1000 {
			inB++
		}
	}

	// A and B have equal area, so an unbiased pick lands in B ~50% of the
	// time. Under the old per-territory Z check, B's own [-500,-500] never
	// accepts z=500, so B only survives via full-budget exhaustion
	// (probability ~0.5^10 ≈ 0.1%). The merged range accepts B immediately,
	// so a fixed implementation should land in B close to 50% of the time.
	if frac := float64(inB) / trials; frac < 0.20 {
		t.Fatalf("randomTerritoryPosition landed in territory B %.3f of the time, want >0.20 (old per-territory Z check would give ~0.001)", frac)
	}
}
