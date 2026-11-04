package pathfind

import (
	"hash/fnv"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// escortPathFingerprint pins every Find/FindInto/HasPath answer over
// escortCases, captured before the search's cell reads were cached.
const escortPathFingerprint = 0xec32669a74fcae65

// escortCase is one raid-boss private's escort search as a fresh server
// boot runs it: origin is the private's position, target the escort slot
// after the move layer snapped it.
type escortCase struct {
	name string
	pathCase
	// exhausts marks the searches that run all MaxIterations with both
	// frontiers still open.
	exhausts bool
}

// escortCases were logged from three local boots on the shared datapack and
// geodata: every search an escorting private ran that allocated over 1000
// nodes or took over 1 ms. The slots sit on the master's Z, and a slot over
// another floor makes the search fail.
var escortCases = []escortCase{
	{name: "25062_18_23", pathCase: pathCase{origin: location.Location{X: -60223, Y: 188032, Z: -4520}, target: location.Location{X: -60362, Y: 188007, Z: -3232}}, exhausts: true},
	{name: "25061_18_23", pathCase: pathCase{origin: location.Location{X: -60284, Y: 188032, Z: -4520}, target: location.Location{X: -60362, Y: 188007, Z: -3232}}, exhausts: true},
	{name: "25180_25_19", pathCase: pathCase{origin: location.Location{X: 182159, Y: 52328, Z: -4360}, target: location.Location{X: 182225, Y: 52329, Z: -3464}}, exhausts: true},
	{name: "25221_23_18", pathCase: pathCase{origin: location.Location{X: 111926, Y: 16772, Z: -2120}, target: location.Location{X: 111843, Y: 16712, Z: -2920}}, exhausts: true},
	{name: "25030_21_24", pathCase: pathCase{origin: location.Location{X: 56543, Y: 205852, Z: -3680}, target: location.Location{X: 56547, Y: 205748, Z: -3568}}},
	{name: "25031_21_24", pathCase: pathCase{origin: location.Location{X: 56447, Y: 205773, Z: -3688}, target: location.Location{X: 56547, Y: 205748, Z: -3568}}},
}

// escortTileMargin is how close, in world units, an endpoint may come to a
// region edge before the neighboring region is loaded too.
// TestFindEscortResultsMatchCapturedFingerprint proves no search leaves the
// loaded regions.
const escortTileMargin = 512 * 16

func escortTile(x, y int) [2]int {
	return [2]int{(x-engine.WorldXMin)/engine.TileSize + engine.TileXMin, (y-engine.WorldYMin)/engine.TileSize + engine.TileYMin}
}

// escortTiles lists the regions the escort searches need.
func escortTiles() [][2]int {
	var tiles [][2]int
	for _, c := range escortCases {
		for _, p := range []location.Location{c.origin, c.target} {
			for _, dx := range []int{-escortTileMargin, 0, escortTileMargin} {
				for _, dy := range []int{-escortTileMargin, 0, escortTileMargin} {
					if tile := escortTile(p.X+dx, p.Y+dy); !slices.Contains(tiles, tile) {
						tiles = append(tiles, tile)
					}
				}
			}
		}
	}
	return tiles
}

func loadEscortSample(t testing.TB) (*engine.Engine, [][2]int) {
	t.Helper()
	tiles := escortTiles()
	return loadGeodataTiles(t, tiles), tiles
}

// searchNodes runs c's search the way find does and returns the scratch
// holding every node it allocated, or nil when find answers before
// searching.
func searchNodes(f *Finder, c pathCase) *searchScratch {
	scratch := &searchScratch{}
	scratch.reset()
	start := scratch.newNodeFromWorld(c.origin.X, c.origin.Y, int(f.engine.Height(c.origin.X, c.origin.Y, c.origin.Z)))
	start.nswe = f.engine.NSWENearest(start.gx, start.gy, start.z)
	goal := scratch.newNodeFromWorld(c.target.X, c.target.Y, int(f.engine.Height(c.target.X, c.target.Y, c.target.Z)))
	goal.nswe = f.engine.NSWENearest(goal.gx, goal.gy, goal.z)
	if gap := goal.z - c.target.Z; gap > maxTargetHeightGap || -gap > maxTargetHeightGap || start.key() == goal.key() {
		return nil
	}
	f.findBidirectional(nil, start, goal, false, scratch)
	return scratch
}

// TestFindEscortResultsMatchCapturedFingerprint pins the escort searches'
// answers, and checks the sample still exercises what it was logged for:
// each search stays inside the loaded regions, and the exhausting ones
// still run to MaxIterations.
func TestFindEscortResultsMatchCapturedFingerprint(t *testing.T) {
	if testing.Short() {
		t.Skip("real-geodata sample skipped in -short")
	}
	e, tiles := loadEscortSample(t)
	f := New(e, DefaultOptions())
	cases := make([]pathCase, len(escortCases))
	for i, c := range escortCases {
		cases[i] = c.pathCase
		if got := exhaustsIterations(f, c.pathCase); got != c.exhausts {
			t.Errorf("%s: exhausts MaxIterations = %v, want %v", c.name, got, c.exhausts)
		}
		scratch := searchNodes(f, c.pathCase)
		if scratch == nil {
			t.Fatalf("%s: search answered before expanding", c.name)
		}
		for id := int32(1); id <= scratch.nodes.n; id++ {
			n := scratch.nodes.at(id)
			if tile := escortTile(engine.WorldX(n.gx), engine.WorldY(n.gy)); !slices.Contains(tiles, tile) {
				t.Fatalf("%s: search reached region %v, outside the loaded %v", c.name, tile, tiles)
			}
		}
	}

	h := fnv.New64a()
	found := pathFingerprint(t, h, f, cases)
	t.Logf("escort sample: %d cases over regions %v, %d found, fingerprint %#x", len(cases), tiles, found, h.Sum64())
	if got := h.Sum64(); got != escortPathFingerprint {
		t.Fatalf("escort path fingerprint = %#x, want %#x", got, uint64(escortPathFingerprint))
	}
}

// BenchmarkFinderEscort measures FindInto on each logged escort search.
func BenchmarkFinderEscort(b *testing.B) {
	e, _ := loadEscortSample(b)
	f := New(e, DefaultOptions())
	var all []pathCase
	for _, c := range escortCases {
		benchmarkCases(b, f, c.name, []pathCase{c.pathCase})
		all = append(all, c.pathCase)
	}
	benchmarkCases(b, f, "all", all)
}
