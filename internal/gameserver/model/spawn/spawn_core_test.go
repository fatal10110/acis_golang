package spawn

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/geometry"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// ---- from entry_test.go ----
func TestParsePositionsAllowsTrailingWeightedSeparator(t *testing.T) {
	positions, err := ParsePositions("1;2;3;4;60%;5;6;7;8;40%;")
	if err != nil {
		t.Fatalf("ParsePositions error: %v", err)
	}
	if got, want := len(positions), 2; got != want {
		t.Fatalf("len(positions) = %d, want %d", got, want)
	}
	if got, want := positions[1].Chance, 40; got != want {
		t.Fatalf("positions[1].Chance = %d, want %d", got, want)
	}
	if got := positions[0]; got.Location.X != 1 || got.Location.Y != 2 || got.Location.Z != 3 || got.Heading != 4 {
		t.Fatalf("positions[0] = %+v, want x/y/z/heading 1/2/3/4", got)
	}
}

func TestParsePositionsRejectsMalformedCoordinate(t *testing.T) {
	if _, err := ParsePositions("1;x;3;4;60%"); err == nil {
		t.Fatal("ParsePositions error = nil, want a parse failure for a non-numeric y")
	}
}

// Java (SpawnManager.java:205-213) sizes the weighted array at
// loc.length/5 and loops only over complete groups: a trailing partial
// group is silently dropped without being parsed, even if malformed.
func TestParsePositionsDropsIncompleteTrailingWeightedGroup(t *testing.T) {
	positions, err := ParsePositions("1;2;3;4;60%;garbage")
	if err != nil {
		t.Fatalf("ParsePositions error: %v", err)
	}
	if got, want := len(positions), 1; got != want {
		t.Fatalf("len(positions) = %d, want %d", got, want)
	}
	if got := positions[0]; got.Location.X != 1 || got.Location.Y != 2 || got.Location.Z != 3 || got.Heading != 4 || got.Chance != 60 {
		t.Fatalf("positions[0] = %+v, want x/y/z/heading/chance 1/2/3/4/60", got)
	}
}

func TestParsePositionsFloorsGroupCountForNonMultipleTokenCounts(t *testing.T) {
	// 9 tokens: one complete weighted group (5) plus a dropped partial (4).
	positions, err := ParsePositions("1;2;3;4;10%;5;6;7;8")
	if err != nil {
		t.Fatalf("ParsePositions error: %v", err)
	}
	if got, want := len(positions), 1; got != want {
		t.Fatalf("len(positions) = %d, want %d", got, want)
	}
}

func TestParsePositionsRejectsFewerThanFourTokens(t *testing.T) {
	if _, err := ParsePositions("1;2;3"); err == nil {
		t.Fatal("ParsePositions error = nil, want a failure for an incomplete fixed tuple")
	}
}

// ---- from respawn_test.go ----
func TestCalculateRespawnDelayNoRespawnWhenDelayIsZero(t *testing.T) {
	entry := Entry{RespawnDelay: 0, RespawnRandom: 5 * time.Second}
	if got := CalculateRespawnDelay(entry); got != 0 {
		t.Fatalf("CalculateRespawnDelay() = %v, want 0", got)
	}
}

func TestCalculateRespawnDelayNoRandomReturnsDelay(t *testing.T) {
	entry := Entry{RespawnDelay: 30 * time.Second, RespawnRandom: 0}
	if got := CalculateRespawnDelay(entry); got != 30*time.Second {
		t.Fatalf("CalculateRespawnDelay() = %v, want 30s", got)
	}
}

func TestCalculateRespawnDelayStaysWithinBounds(t *testing.T) {
	entry := Entry{RespawnDelay: 30 * time.Second, RespawnRandom: 10 * time.Second}
	min, max := 20*time.Second, 40*time.Second

	for i := 0; i < 500; i++ {
		got := CalculateRespawnDelay(entry)
		if got < min || got > max {
			t.Fatalf("CalculateRespawnDelay() = %v, want within [%v, %v]", got, min, max)
		}
	}
}

func TestCalculateRespawnDelayClampsRandomToDelay(t *testing.T) {
	// RespawnRandom larger than RespawnDelay must clamp so the result never
	// goes negative, matching the reference implementation's guarantee.
	entry := Entry{RespawnDelay: 5 * time.Second, RespawnRandom: 50 * time.Second}

	for i := 0; i < 500; i++ {
		got := CalculateRespawnDelay(entry)
		if got < 0 || got > 10*time.Second {
			t.Fatalf("CalculateRespawnDelay() = %v, want within [0, 10s]", got)
		}
	}
}

// ---- from state_test.go ----
func TestStateLifecycle(t *testing.T) {
	now := time.UnixMilli(1_000)
	state := NewState("boss_1")

	if state.Status != StatusUninitialized {
		t.Fatalf("new state status = %d, want %d", state.Status, StatusUninitialized)
	}

	loc := location.Location{X: 10, Y: 20, Z: 30}
	if kept := state.CheckAlive(loc, 40, 500, 200, now); kept {
		t.Fatal("CheckAlive() for uninitialized state = true, want false")
	}
	if state.Status != StatusAlive || state.CurrentHP != 500 || state.CurrentMP != 200 || state.Location != loc || state.Heading != 40 || state.RespawnTime != 0 {
		t.Fatalf("state after CheckAlive() = %+v", state)
	}

	state.SetRespawn(2*time.Second, now)
	if state.Status != StatusDead || state.CurrentHP != 0 || state.CurrentMP != 0 || state.Location != (location.Location{}) || state.Heading != 0 {
		t.Fatalf("state after SetRespawn() = %+v", state)
	}
	if state.RespawnTime != 3_000 {
		t.Fatalf("RespawnTime = %d, want 3000", state.RespawnTime)
	}
	if !state.Dead(now.Add(1999 * time.Millisecond)) {
		t.Fatal("Dead() before respawn time = false, want true")
	}
	if state.Dead(now.Add(2 * time.Second)) {
		t.Fatal("Dead() at respawn time = true, want false")
	}

	state.CancelRespawn()
	if state.RespawnTime != 1 {
		t.Fatalf("CancelRespawn() respawn time = %d, want 1", state.RespawnTime)
	}
}

func TestStateCheckAliveRestoresExistingRow(t *testing.T) {
	now := time.UnixMilli(1_000)
	state := &State{
		Name:      "queen_ant",
		Status:    StatusAlive,
		CurrentHP: 123,
		CurrentMP: 45,
		Location:  location.Location{X: 1, Y: 2, Z: 3},
		Heading:   4,
	}

	if kept := state.CheckAlive(location.Location{X: 10, Y: 20, Z: 30}, 40, 500, 200, now); !kept {
		t.Fatal("CheckAlive() for persisted alive state = false, want true")
	}
	if state.CurrentHP != 123 || state.CurrentMP != 45 || state.Location != (location.Location{X: 1, Y: 2, Z: 3}) || state.Heading != 4 {
		t.Fatalf("persisted alive state was overwritten: %+v", state)
	}
}

func TestStateSetStatsSkipsDeadRows(t *testing.T) {
	state := &State{
		Name:        "dead_boss",
		Status:      StatusDead,
		RespawnTime: 9_000,
	}

	state.SetStats(10, 20, location.Location{X: 1, Y: 2, Z: 3}, 4)
	if state.CurrentHP != 0 || state.CurrentMP != 0 || state.Location != (location.Location{}) || state.Heading != 0 || state.RespawnTime != 9_000 {
		t.Fatalf("dead state changed after SetStats(): %+v", state)
	}
}

// ---- from territory_test.go ----
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
	if !tr.Contains(1, 1, 0) {
		t.Error("literal-constructed Territory does not contain an interior point")
	}
	if got, want := tr.Area(), 50.0; got != want {
		t.Errorf("literal-constructed Territory Area() = %v, want %v", got, want)
	}
	other, err := geometry.NewTerritory(-100, 100, geometry.NewRectangle(0, 1, 0, 1))
	if err != nil {
		t.Fatalf("NewTerritory: %v", err)
	}
	if !tr.Intersects(other) {
		t.Error("literal-constructed Territory does not intersect an overlapping territory")
	}
}
