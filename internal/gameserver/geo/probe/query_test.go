package probe

import (
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/pathfind"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// openLoadedEngine returns an engine with one loaded block, at
// (TileXMin, TileYMin), open and flat (height 0, every direction passable)
// across all its cells, plus a Finder over it with default options.
func openLoadedEngine(t *testing.T) (*engine.Engine, *pathfind.Finder) {
	t.Helper()

	var cells [block.CellCount]block.Cell
	for i := range cells {
		cells[i] = block.Cell{Height: 0, NSWE: block.AllDirections}
	}
	region, err := block.NewRegionFromBlocks([]block.Block{block.NewComplex(cells)})
	if err != nil {
		t.Fatalf("NewRegionFromBlocks(): %v", err)
	}

	e := engine.New()
	if err := e.SetRegion(engine.TileXMin, engine.TileYMin, region); err != nil {
		t.Fatalf("SetRegion(): %v", err)
	}
	return e, pathfind.New(e, pathfind.DefaultOptions())
}

func TestQueryIDRoundTrip(t *testing.T) {
	tests := []Query{
		{Category: Height, From: location.Location{X: 1, Y: -2, Z: 3}},
		{Category: CanMove, From: location.Location{X: 1, Y: 2, Z: 3}, To: location.Location{X: -4, Y: 5, Z: -6}},
		{Category: LineOfSight, From: location.Location{X: 1, Y: 2, Z: 3}, To: location.Location{X: 4, Y: 5, Z: 6}},
		{Category: Path, From: location.Location{X: 1, Y: 2, Z: 3}, To: location.Location{X: 4, Y: 5, Z: 6}},
	}

	for _, q := range tests {
		got, err := ParseQuery(q.ID())
		if err != nil {
			t.Errorf("ParseQuery(%q): %v", q.ID(), err)
			continue
		}
		if got != q {
			t.Errorf("ParseQuery(%q) = %#v, want %#v", q.ID(), got, q)
		}
	}
}

func TestParseQueryErrors(t *testing.T) {
	tests := []string{
		"",
		"height",
		"height:1,2",
		"height:x,2,3",
		"canmove:1,2,3",
		"canmove:1,2,3->4,5",
		"bogus:1,2,3->4,5,6",
		"height:1,2,99999999999",
	}
	for _, id := range tests {
		if _, err := ParseQuery(id); err == nil {
			t.Errorf("ParseQuery(%q) error = nil, want error", id)
		}
	}
}

func TestEvaluate(t *testing.T) {
	e := engine.New()
	finder := pathfind.New(e, pathfind.DefaultOptions())

	x, y := engine.WorldXMin, engine.WorldYMin

	t.Run("height", func(t *testing.T) {
		r := Evaluate(e, finder, Query{Category: Height, From: location.Location{X: x, Y: y, Z: 42}})
		if r.ID != "height:"+formatPoint(location.Location{X: x, Y: y, Z: 42}) {
			t.Errorf("ID = %q", r.ID)
		}
		if r.Fields["height"] != "42" {
			t.Errorf("height = %q, want 42 (null block echoes queried Z)", r.Fields["height"])
		}
	})

	t.Run("canmove same cell", func(t *testing.T) {
		from := location.Location{X: x, Y: y, Z: 0}
		r := Evaluate(e, finder, Query{Category: CanMove, From: from, To: from})
		if r.Fields["result"] != "true" {
			t.Errorf("result = %q, want true", r.Fields["result"])
		}
	})

	t.Run("los", func(t *testing.T) {
		from := location.Location{X: x, Y: y, Z: 0}
		to := location.Location{X: x + 32, Y: y, Z: 0}
		r := Evaluate(e, finder, Query{Category: LineOfSight, From: from, To: to})
		if r.Fields["result"] != "true" {
			t.Errorf("result = %q, want true over open null geodata", r.Fields["result"])
		}
	})

	t.Run("path", func(t *testing.T) {
		// A pure Null engine can't be pathfound across: Null.HeightNearest
		// echoes the queried Z while Null.Height always answers 0, so a
		// step's post-move height check never matches. Real deployments
		// always load real geodata for reachable regions, so use an open
		// loaded block here instead of exercising that Null-only edge case.
		e, finder := openLoadedEngine(t)
		x, y := engine.WorldX(0), engine.WorldY(0)
		from := location.Location{X: x, Y: y, Z: 0}
		to := location.Location{X: x + 48, Y: y, Z: 0}
		r := Evaluate(e, finder, Query{Category: Path, From: from, To: to})
		if r.Fields["found"] != "true" {
			t.Fatalf("found = %q, want true over an open loaded block", r.Fields["found"])
		}
		if r.Fields["cost"] == "" {
			t.Error("cost is empty, want a value when found")
		}
		if r.Fields["points"] == "" {
			t.Error("points is empty, want a value when found")
		}
	})
}

func TestRandomIsDeterministic(t *testing.T) {
	a := Random(40, 7)
	b := Random(40, 7)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("Random() with the same seed produced different query sets")
	}

	c := Random(40, 8)
	if reflect.DeepEqual(a, c) {
		t.Fatal("Random() with different seeds produced the same query set")
	}
}

func TestRandomCyclesCategories(t *testing.T) {
	queries := Random(len(categories)*3, 1)
	for i, q := range queries {
		if want := categories[i%len(categories)]; q.Category != want {
			t.Fatalf("queries[%d].Category = %q, want %q", i, q.Category, want)
		}
	}
}

func TestRandomStaysInWorldBounds(t *testing.T) {
	for _, q := range Random(200, 3) {
		for _, p := range []location.Location{q.From, q.To} {
			if engine.OutOfWorld(p.X, p.Y) {
				t.Fatalf("query %v has out-of-world point %v", q, p)
			}
		}
	}
}
