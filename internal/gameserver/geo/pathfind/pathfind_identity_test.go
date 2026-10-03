package pathfind

import (
	"encoding/binary"
	"fmt"
	"hash"
	"hash/fnv"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/reader"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// The fingerprints below were captured from the search before its open/closed
// sets moved off Go maps. Every Find/FindInto/HasPath answer (found, cost and
// every waypoint) over each sample folds into one hash, so any change in a
// path, a cost or a MaxIterations cutoff changes it.
const (
	syntheticPathFingerprint = 0xd065b846733691c7
	geodataPathFingerprint   = 0xe6335cc8764b1bd5
)

type pathCase struct {
	origin, target location.Location
}

// pathFingerprint runs every case through Find, FindInto (into one reused
// buffer) and HasPath, fails on any disagreement between them, and folds the
// Find answers into h.
func pathFingerprint(t testing.TB, h hash.Hash64, f *Finder, cases []pathCase) (found int) {
	t.Helper()

	var dst []location.Location
	var word [8]byte
	put := func(v int) {
		binary.LittleEndian.PutUint64(word[:], uint64(int64(v)))
		h.Write(word[:])
	}
	for i, c := range cases {
		path, cost, ok := f.Find(c.origin, c.target)
		into, intoCost, intoOK := f.FindInto(dst[:0], c.origin, c.target)
		dst = into
		if intoOK != ok || intoCost != cost || len(into) != len(path) {
			t.Fatalf("case %d: FindInto = (%d points, %d, %v), Find = (%d points, %d, %v)", i, len(into), intoCost, intoOK, len(path), cost, ok)
		}
		for j := range path {
			if into[j] != path[j] {
				t.Fatalf("case %d: FindInto point %d = %#v, Find = %#v", i, j, into[j], path[j])
			}
		}
		if has := f.HasPath(c.origin, c.target); has != ok {
			t.Fatalf("case %d: HasPath = %v, Find ok = %v", i, has, ok)
		}

		if ok {
			found++
			put(1)
		} else {
			put(0)
		}
		put(cost)
		put(len(path))
		for _, p := range path {
			put(p.X)
			put(p.Y)
			put(p.Z)
		}
	}
	return found
}

// splitMazeEngine is a width x height open grid with seeded wall segments, a
// solid wall splitting it at x=split, so pairs straddling it are unreachable
// and both frontiers have room to exhaust MaxIterations.
func splitMazeEngine(t testing.TB, width, height, split int, seed int64) *engine.Engine {
	t.Helper()

	r := rand.New(rand.NewSource(seed))
	walls := make(map[[2]int]bool)
	for i := 0; i < width*height/40; i++ {
		x, y := r.Intn(width), r.Intn(height)
		horizontal := r.Intn(2) == 0
		for k := 0; k < 3+r.Intn(10); k++ {
			walls[[2]int{x, y}] = true
			if horizontal {
				x++
			} else {
				y++
			}
		}
	}
	return newGridEngine(t, width, height, func(x, y int) block.Cell {
		if x == split || walls[[2]int{x, y}] {
			return block.Cell{Height: 0, NSWE: block.NoDirections}
		}
		return block.Cell{Height: int16((x / 16) * 8), NSWE: block.AllDirections}
	})
}

func randomGridCases(r *rand.Rand, width, height, maxSpan, n int) []pathCase {
	cases := make([]pathCase, n)
	for i := range cases {
		ox, oy := r.Intn(width), r.Intn(height)
		tx := min(max(ox+r.Intn(2*maxSpan+1)-maxSpan, 0), width-1)
		ty := min(max(oy+r.Intn(2*maxSpan+1)-maxSpan, 0), height-1)
		cases[i] = pathCase{origin: at(ox, oy, 0), target: at(tx, ty, 0)}
	}
	return cases
}

func syntheticPathSuite(t testing.TB, h hash.Hash64) (cases, found int) {
	t.Helper()

	const width, height, split = 192, 160, 96
	for seed := int64(1); seed <= 2; seed++ {
		e := splitMazeEngine(t, width, height, split, seed)
		r := rand.New(rand.NewSource(seed * 7919))
		sample := randomGridCases(r, width, height, 120, 50)
		for _, bidirectional := range []bool{true, false} {
			for _, maxIterations := range []int{DefaultOptions().MaxIterations, 300} {
				options := DefaultOptions()
				options.Bidirectional = bidirectional
				options.MaxIterations = maxIterations
				found += pathFingerprint(t, h, New(e, options), sample)
				cases += len(sample)
			}
		}
	}

	for seed := int64(1); seed <= 8; seed++ {
		r := rand.New(rand.NewSource(seed))
		var cells [block.CellCount][]block.Cell
		for x := range block.CellsX {
			for y := range block.CellsY {
				layers := make([]block.Cell, r.Intn(3)+1)
				height := int16(0)
				for i := range layers {
					height += int16(r.Intn(4) * 16)
					nswe := block.AllDirections
					if r.Intn(5) == 0 {
						nswe = block.NSWE(r.Intn(16))
					}
					layers[i] = block.Cell{Height: height, NSWE: nswe}
				}
				cells[x*block.CellsY+y] = layers
			}
		}
		ml, err := block.NewMultilayer(cells)
		if err != nil {
			t.Fatalf("NewMultilayer(): %v", err)
		}
		e := newTestEngine(t, ml)
		top := func(x, y int) int {
			layers := cells[x*block.CellsY+y]
			return int(layers[len(layers)-1].Height)
		}
		sample := []pathCase{
			{origin: at(0, 0, top(0, 0)), target: at(7, 7, top(7, 7))},
			{origin: at(7, 0, top(7, 0)), target: at(0, 7, 0)},
			{origin: at(3, 0, 0), target: at(4, 7, top(4, 7))},
		}
		for _, bidirectional := range []bool{true, false} {
			options := DefaultOptions()
			options.Bidirectional = bidirectional
			found += pathFingerprint(t, h, New(e, options), sample)
			cases += len(sample)
		}
	}
	return cases, found
}

// TestFindResultsMatchCapturedFingerprint pins every search answer over a
// mixed sample — routed, unreachable, MaxIterations-capped, multilayer, both
// search modes — to the answers captured before the search's set storage
// changed.
func TestFindResultsMatchCapturedFingerprint(t *testing.T) {
	h := fnv.New64a()
	cases, found := syntheticPathSuite(t, h)
	t.Logf("synthetic sample: %d cases, %d found, fingerprint %#x", cases, found, h.Sum64())
	if found == 0 || found == cases {
		t.Fatalf("synthetic sample found %d/%d paths, want a mix of found and failed", found, cases)
	}
	if got := h.Sum64(); got != syntheticPathFingerprint {
		t.Fatalf("synthetic path fingerprint = %#x, want %#x", got, uint64(syntheticPathFingerprint))
	}
}

// geodataRegions are the L2OFF regions around Gludio castle the real-geodata
// sample and benchmarks load.
var geodataRegions = [][2]int{{19, 21}, {19, 22}, {20, 21}, {20, 22}}

// loadGeodataSample loads geodataRegions from the shared datapack, skipping
// when the geodata files are not present (they are not in the repository).
func loadGeodataSample(t testing.TB) *engine.Engine {
	t.Helper()

	dir := os.Getenv("ACIS_GEODATA_DIR")
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			t.Fatalf("Getwd(): %v", err)
		}
		for d := wd; ; d = filepath.Dir(d) {
			candidate := filepath.Join(d, "aCis_datapack", "data", "geodata")
			if _, err := os.Stat(candidate); err == nil {
				dir = candidate
				break
			}
			if filepath.Dir(d) == d {
				break
			}
		}
	}
	if dir == "" {
		t.Skip("no aCis_datapack/data/geodata above the package and ACIS_GEODATA_DIR unset")
	}

	e := engine.New()
	for _, tile := range geodataRegions {
		path := filepath.Join(dir, filepathRegion(tile))
		if _, err := os.Stat(path); err != nil {
			t.Skipf("geodata region %s missing: %v", path, err)
		}
		region, _, err := reader.ReadL2OFF(path)
		if err != nil {
			t.Fatalf("ReadL2OFF(%s): %v", path, err)
		}
		if err := e.SetRegion(tile[0], tile[1], region); err != nil {
			t.Fatalf("SetRegion(%v): %v", tile, err)
		}
	}
	return e
}

func filepathRegion(tile [2]int) string {
	return fmt.Sprintf("%d_%d_conv.dat", tile[0], tile[1])
}

// geodataCases draws n origin/target pairs inside the loaded regions with
// targets up to span world units from their origin, each snapped to a
// geodata layer, and splits them into the ones a straight move reaches and
// the ones that need a search.
func geodataCases(e *engine.Engine, seed int64, n, span int) (direct, searched []pathCase) {
	r := rand.New(rand.NewSource(seed))
	minX := engine.WorldXMin + (geodataRegions[0][0]-engine.TileXMin)*engine.TileSize
	minY := engine.WorldYMin + (geodataRegions[0][1]-engine.TileYMin)*engine.TileSize
	const regionSpan = 2 * engine.TileSize
	point := func(x, y int) location.Location {
		z := r.Intn(8001) - 4000
		return location.Location{X: x, Y: y, Z: int(e.Height(x, y, z))}
	}
	for len(direct)+len(searched) < n {
		ox := minX + r.Intn(regionSpan)
		oy := minY + r.Intn(regionSpan)
		tx := min(max(ox+r.Intn(2*span+1)-span, minX), minX+regionSpan-1)
		ty := min(max(oy+r.Intn(2*span+1)-span, minY), minY+regionSpan-1)
		c := pathCase{origin: point(ox, oy), target: point(tx, ty)}
		if e.CanMove(c.origin.X, c.origin.Y, c.origin.Z, c.target.X, c.target.Y, c.target.Z) {
			direct = append(direct, c)
		} else {
			searched = append(searched, c)
		}
	}
	return direct, searched
}

// TestFindGeodataResultsMatchCapturedFingerprint is the real-terrain
// counterpart of TestFindResultsMatchCapturedFingerprint. It runs only where
// the shared datapack's geodata is present.
func TestFindGeodataResultsMatchCapturedFingerprint(t *testing.T) {
	if testing.Short() {
		t.Skip("real-geodata sample skipped in -short")
	}
	e := loadGeodataSample(t)
	direct, searched := geodataCases(e, 1, 1500, 2000)
	h := fnv.New64a()
	found := pathFingerprint(t, h, New(e, DefaultOptions()), append(direct, searched...))
	t.Logf("geodata sample: %d direct, %d searched, %d found, fingerprint %#x", len(direct), len(searched), found, h.Sum64())
	if got := h.Sum64(); got != geodataPathFingerprint {
		t.Fatalf("geodata path fingerprint = %#x, want %#x", got, uint64(geodataPathFingerprint))
	}
}
