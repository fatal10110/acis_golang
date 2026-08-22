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
func (constGeoZ) Walkable(int, int, int) bool { return true }

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

// halfWalkableGeo models a territory whose geodata straddles walkable and
// unwalkable ground: every point with X < unwalkableMaxX is rejected, same
// as Territory.getRandomLocation's canMoveAround retry in the Java
// reference (aCis_gameserver Territory.java:138/193).
type halfWalkableGeo struct {
	unwalkableMaxX int
	walkableCalls  int
}

func (halfWalkableGeo) CanMove(int, int, int, int, int, int) bool { return true }
func (halfWalkableGeo) Height(_, _, z int) int16                  { return int16(z) }
func (halfWalkableGeo) FindPath(_, _ location.Location) ([]location.Location, bool) {
	return nil, false
}
func (halfWalkableGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

func (g *halfWalkableGeo) Walkable(x, _, _ int) bool {
	g.walkableCalls++
	return x >= g.unwalkableMaxX
}

func squareTerritory(minX, minY, maxX, maxY int) *spawn.Territory {
	return &spawn.Territory{
		Name: "half-walkable",
		MinZ: -100,
		MaxZ: 100,
		Nodes: []spawn.Node{
			{X: minX, Y: minY},
			{X: maxX, Y: minY},
			{X: maxX, Y: maxY},
			{X: minX, Y: maxY},
		},
	}
}

// TestRandomTerritoryPosition_RetriesUnwalkablePoints regression-tests #1716:
// a candidate point that fails the walkability check must be retried within
// the existing territorySpawnAttempts budget rather than accepted outright.
// The unwalkable band covers only a fifth of the territory's width, so the
// 10-attempt budget finds a walkable point with overwhelming probability;
// this asserts every returned placement over many runs lands on the
// walkable side.
func TestRandomTerritoryPosition_RetriesUnwalkablePoints(t *testing.T) {
	territory := squareTerritory(0, 0, 100, 100)
	maker := &spawn.Maker{Territories: []*spawn.Territory{territory}}
	geo := &halfWalkableGeo{unwalkableMaxX: 20}

	for i := 0; i < 300; i++ {
		pos, ok := randomTerritoryPosition(maker, geo)
		if !ok {
			t.Fatalf("run %d: randomTerritoryPosition returned ok=false", i)
		}
		if pos.Location.X < 20 {
			t.Fatalf("run %d: placed NPC at unwalkable X=%d, want >= 20", i, pos.Location.X)
		}
	}

	if geo.walkableCalls == 0 {
		t.Fatal("Walkable was never called; randomTerritoryPosition is not checking geodata")
	}
}

// TestRandomTerritoryPosition_FallsBackWhenNeverWalkable matches
// Territory.getRandomLocation's exhaustion behavior: when every candidate
// fails the walkability check, the last rolled position is still returned
// rather than failing placement outright.
func TestRandomTerritoryPosition_FallsBackWhenNeverWalkable(t *testing.T) {
	territory := squareTerritory(0, 0, 100, 100)
	maker := &spawn.Maker{Territories: []*spawn.Territory{territory}}
	geo := &halfWalkableGeo{unwalkableMaxX: 1000} // rejects every point

	pos, ok := randomTerritoryPosition(maker, geo)
	if !ok {
		t.Fatal("randomTerritoryPosition returned ok=false, want fallback position")
	}
	if pos.Location.X < 0 || pos.Location.X > 100 {
		t.Fatalf("fallback position X=%d outside territory bounds", pos.Location.X)
	}
	if geo.walkableCalls != territorySpawnAttempts {
		t.Fatalf("walkableCalls = %d, want %d (one per attempt, all exhausted)", geo.walkableCalls, territorySpawnAttempts)
	}
}
