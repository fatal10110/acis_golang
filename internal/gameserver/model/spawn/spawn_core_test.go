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

// TestTerritoryContainsUsesTriangulationNotRayCasting pins the containment
// rule spawn placement runs on, using a self-touching territory taken
// verbatim from the shipped spawnlist ("godard01_npc2315_04",
// spawnlist/23_15.xml: the four corners of a 200x200 box listed in bowtie
// order). Even-odd ray casting over the same node ring puts this point
// outside, so the assertion fails if the ray-cast path comes back.
func TestTerritoryContainsUsesTriangulationNotRayCasting(t *testing.T) {
	nodes := []Node{
		{X: 122400, Y: -69800},
		{X: 122400, Y: -70000},
		{X: 122600, Y: -69800},
		{X: 122600, Y: -70000},
	}
	set := commons.NewStatSet()
	set.Set("name", "godard01_npc2315_04")
	set.Set("minZ", "-3250")
	set.Set("maxZ", "-3100")

	territory, err := NewTerritory(set, nodes)
	if err != nil {
		t.Fatalf("NewTerritory() error = %v", err)
	}

	const x, y = 122500, -69850
	if !territory.Contains2D(x, y) {
		t.Errorf("Contains2D(%d, %d) = false, want true", x, y)
	}
	if !territory.Contains(x, y, -3200) {
		t.Errorf("Contains(%d, %d, -3200) = false, want true", x, y)
	}
	if territory.Contains(x, y, -3000) {
		t.Error("Contains(z above maxZ) = true, want false")
	}

	ring, err := geometry.NewPolygon([]geometry.Point{
		{X: 122400, Y: -69800},
		{X: 122400, Y: -70000},
		{X: 122600, Y: -69800},
		{X: 122600, Y: -70000},
	})
	if err != nil {
		t.Fatalf("NewPolygon() error = %v", err)
	}
	if ring.Contains(x, y) {
		t.Errorf("even-odd ray casting also contains (%d, %d); this fixture only "+
			"proves anything while the two algorithms disagree there", x, y)
	}

	// A territory built as a struct literal has no prebuilt geometry; it must
	// reach the same answer by triangulating its nodes on demand.
	literal := &Territory{Name: "literal", MinZ: -3250, MaxZ: -3100, Nodes: nodes}
	if !literal.Contains2D(x, y) {
		t.Errorf("literal territory Contains2D(%d, %d) = false, want true", x, y)
	}
}

// ---- territory-random sampling ----

func mustTriangle(t *testing.T, a, b, c geometry.Point) geometry.Triangle {
	t.Helper()
	tri, err := geometry.NewTriangle(a, b, c)
	if err != nil {
		t.Fatalf("NewTriangle: %v", err)
	}
	return tri
}

// TestPickTriangleWalksCumulativeSizes pins Territory.getRandomLocation's
// triangle pick: rand = Rnd.get(size) and each triangle's size is
// subtracted in order until rand drops below zero. Sizes are 50, 5000, 50.
func TestPickTriangleWalksCumulativeSizes(t *testing.T) {
	first := mustTriangle(t, geometry.Point{X: 0, Y: 0}, geometry.Point{X: 10, Y: 0}, geometry.Point{X: 0, Y: 10})
	big := mustTriangle(t, geometry.Point{X: 0, Y: 0}, geometry.Point{X: 100, Y: 0}, geometry.Point{X: 0, Y: 100})
	last := mustTriangle(t, geometry.Point{X: 900, Y: 900}, geometry.Point{X: 910, Y: 900}, geometry.Point{X: 900, Y: 910})
	triangles := []geometry.Triangle{first, big, last}

	for _, tt := range []struct {
		roll int64
		want geometry.Triangle
	}{
		{0, first}, {49, first}, {50, big}, {5049, big}, {5050, last}, {5099, last},
	} {
		if got := pickTriangle(triangles, tt.roll); got.Center() != tt.want.Center() {
			t.Errorf("pickTriangle(roll=%d) center = %+v, want %+v", tt.roll, got.Center(), tt.want.Center())
		}
	}
}

type flatTerrain struct {
	z        int16
	walkable func(x, y int) bool
}

func (g flatTerrain) Height(int, int, int) int16 { return g.z }
func (g flatTerrain) Walkable(x, y, _ int) bool {
	return g.walkable == nil || g.walkable(x, y)
}

func triangleTerritory(name string, minZ, maxZ int, nodes ...Node) *Territory {
	return &Territory{Name: name, MinZ: minZ, MaxZ: maxZ, Nodes: nodes}
}

// TestMakerRandomLocationDrawsFromMergedTerritory checks the sampler shared
// by territory spawn and out-of-territory wander: every draw lands in a
// member footprint, Z comes from geodata inside the merged range, and a
// member four times larger receives about four times the draws.
func TestMakerRandomLocationDrawsFromMergedTerritory(t *testing.T) {
	small := triangleTerritory("small", 0, 10, Node{X: 0, Y: 0}, Node{X: 100, Y: 0}, Node{X: 0, Y: 100})
	large := triangleTerritory("large", 90, 200, Node{X: 5000, Y: 0}, Node{X: 5200, Y: 0}, Node{X: 5000, Y: 200})
	maker := &Maker{Territories: []*Territory{small, large}}

	const trials = 5000
	inLarge := 0
	for i := 0; i < trials; i++ {
		loc, ok := maker.RandomLocation(flatTerrain{z: 150}, false)
		if !ok {
			t.Fatalf("RandomLocation ok = false")
		}
		if !maker.Contains(loc) {
			t.Fatalf("RandomLocation = %+v, outside the merged territory", loc)
		}
		if large.Contains2D(loc.X, loc.Y) {
			inLarge++
		}
	}
	// Sizes 5000 and 20000: the large member's expected share is 0.8.
	if frac := float64(inLarge) / trials; frac < 0.75 || frac > 0.85 {
		t.Fatalf("large member share = %.3f, want about 0.8", frac)
	}
}

// TestMakerRandomLocationBannedOnlyWhenAsked: spawn placement avoids the
// merged banned territory, the out-of-territory wander draw does not.
func TestMakerRandomLocationBannedOnlyWhenAsked(t *testing.T) {
	maker := &Maker{
		Territories: []*Territory{
			triangleTerritory("in_ban", 0, 1000, Node{X: 0, Y: 0}, Node{X: 100, Y: 0}, Node{X: 0, Y: 100}),
			triangleTerritory("open", 0, 1000, Node{X: 1000, Y: 0}, Node{X: 1100, Y: 0}, Node{X: 1000, Y: 100}),
		},
		BannedTerritories: []*Territory{
			triangleTerritory("ban_a", 0, 100, Node{X: 0, Y: 0}, Node{X: 100, Y: 0}, Node{X: 0, Y: 100}),
			triangleTerritory("ban_b", 500, 600, Node{X: 5000, Y: 5000}, Node{X: 5100, Y: 5000}, Node{X: 5000, Y: 5100}),
		},
	}
	geo := flatTerrain{z: 550}

	sawBanned := false
	for i := 0; i < 500; i++ {
		loc, ok := maker.RandomLocation(geo, true)
		if !ok || maker.ContainsBanned(loc) {
			t.Fatalf("RandomLocation(excludeBanned) = %+v, %v; want an unbanned point", loc, ok)
		}
		if loc, _ := maker.RandomLocation(geo, false); maker.ContainsBanned(loc) {
			sawBanned = true
		}
	}
	if !sawBanned {
		t.Fatal("RandomLocation without excludeBanned never drew in the banned half")
	}
}

// TestMakerRandomLocationKeepsLastDrawAfterTenFailures: with every draw
// out of Z range the sampler gives up after ten failures and returns the
// last draw rather than nothing.
func TestMakerRandomLocationKeepsLastDrawAfterTenFailures(t *testing.T) {
	maker := &Maker{Territories: []*Territory{
		triangleTerritory("t", 0, 10, Node{X: 0, Y: 0}, Node{X: 100, Y: 0}, Node{X: 0, Y: 100}),
	}}
	loc, ok := maker.RandomLocation(flatTerrain{z: 999}, true)
	if !ok || loc.Z != 999 || !maker.Territories[0].Contains2D(loc.X, loc.Y) {
		t.Fatalf("RandomLocation = %+v, %v; want the last in-footprint draw at z 999", loc, ok)
	}

	calls := 0
	maker.RandomLocation(flatTerrain{z: 5, walkable: func(int, int) bool { calls++; return false }}, false)
	if calls != 10 {
		t.Fatalf("Walkable calls = %d, want 10", calls)
	}
}

func TestMakerRandomLocationWithoutTerritory(t *testing.T) {
	for _, maker := range []*Maker{nil, {}} {
		if _, ok := maker.RandomLocation(flatTerrain{}, true); ok {
			t.Fatalf("RandomLocation on %+v ok = true, want false", maker)
		}
	}
}
