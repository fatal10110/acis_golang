package probe

import (
	"reflect"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/pathfind"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func TestWalkIDRoundTrip(t *testing.T) {
	q := Query{
		Category:   Walk,
		From:       location.Location{X: 1, Y: -2, Z: 3},
		To:         location.Location{X: -4, Y: 5, Z: -6},
		Speed:      115.5,
		StartSpeed: 80,
		Steps:      []time.Duration{100 * time.Millisecond, 250 * time.Millisecond, time.Millisecond},
	}
	const want = "walk:1,-2,3->-4,5,-6;speed=115.5;start=80;steps=100,250,1"
	if got := q.ID(); got != want {
		t.Fatalf("ID() = %q, want %q", got, want)
	}
	got, err := ParseQuery(want)
	if err != nil {
		t.Fatalf("ParseQuery(%q): %v", want, err)
	}
	if !reflect.DeepEqual(got, q) {
		t.Fatalf("ParseQuery(%q) = %#v, want %#v", want, got, q)
	}
}

func TestParseWalkErrors(t *testing.T) {
	for _, id := range []string{
		"walk:1,2,3;speed=1;start=1;steps=100",
		"walk:1,2,3->4,5,6",
		"walk:1,2,3->4,5,6;speed=1;start=1",
		"walk:1,2,3->4,5,6;start=1;steps=100",
		"walk:1,2,3->4,5,6;speed=1;start=1;steps=",
		"walk:1,2,3->4,5,6;speed=1;start=1;steps=100,0",
		"walk:1,2,3->4,5,6;speed=1;start=1;steps=100,-5",
		"walk:1,2,3->4,5,6;speed=x;start=1;steps=100",
		"walk:1,2,3->4,5,6;speed=NaN;start=1;steps=100",
		"walk:1,2,3->4,5,6;speed=1;start=-1;steps=100",
		"walk:1,2,3->4,5,6;speed=1;start=1;steps=100;extra=1",
	} {
		if _, err := ParseQuery(id); err == nil {
			t.Errorf("ParseQuery(%q) error = nil, want error", id)
		}
	}
}

func TestEvaluateWalk(t *testing.T) {
	e, f := openLoadedEngine(t)
	x, y := engine.WorldX(0), engine.WorldY(0)
	from := location.Location{X: x, Y: y}
	to := location.Location{X: x + 100, Y: y}

	t.Run("reaches the destination", func(t *testing.T) {
		q := Query{
			Category: Walk, From: from, To: to, Speed: 200, StartSpeed: 100,
			Steps: repeatStep(100*time.Millisecond, 12),
		}
		r := Evaluate(e, f, q)
		if r.ID != q.ID() {
			t.Fatalf("ID = %q, want %q", r.ID, q.ID())
		}
		if r.Fields["reached"] != "true" || r.Fields["blocked"] != "false" {
			t.Fatalf("fields = %v, want reached", r.Fields)
		}
		if want := formatPoint(location.Location{X: to.X, Y: to.Y}); r.Fields["stop"] != want {
			t.Fatalf("stop = %q, want %q", r.Fields["stop"], want)
		}
	})

	t.Run("start speed covers the first five steps", func(t *testing.T) {
		q := Query{
			Category: Walk, From: from, To: to, Speed: 200, StartSpeed: 100,
			Steps: repeatStep(100*time.Millisecond, 6),
		}
		r := Evaluate(e, f, q)
		// 5 updates of 10 units, then one of 20.
		want := formatPath([]location.Location{
			{X: x + 10, Y: y}, {X: x + 20, Y: y}, {X: x + 30, Y: y}, {X: x + 40, Y: y}, {X: x + 50, Y: y}, {X: x + 70, Y: y},
		})
		if r.Fields["points"] != want {
			t.Fatalf("points = %q, want %q", r.Fields["points"], want)
		}
		if r.Fields["reached"] != "false" || r.Fields["blocked"] != "false" {
			t.Fatalf("fields = %v, want still walking", r.Fields)
		}
	})

	t.Run("stops where the geodata refuses", func(t *testing.T) {
		// Moving east out of the cell 30 units on is closed.
		e, f := wallEngine(t)
		wx, wy := engine.WorldX(0), engine.WorldY(0)
		q := Query{
			Category: Walk, From: location.Location{X: wx, Y: wy}, To: location.Location{X: wx + 100, Y: wy},
			Speed: 200, StartSpeed: 100, Steps: repeatStep(100*time.Millisecond, 10),
		}
		r := Evaluate(e, f, q)
		if r.Fields["blocked"] != "true" || r.Fields["reached"] != "false" {
			t.Fatalf("fields = %v, want blocked", r.Fields)
		}
		if want := formatPoint(location.Location{X: wx + 30, Y: wy}); r.Fields["stop"] != want {
			t.Fatalf("stop = %q, want %q", r.Fields["stop"], want)
		}
		// The refused update leaves the walker where it stood and ends the chain.
		want := formatPath([]location.Location{{X: wx + 10, Y: wy}, {X: wx + 20, Y: wy}, {X: wx + 30, Y: wy}, {X: wx + 30, Y: wy}})
		if r.Fields["points"] != want {
			t.Fatalf("points = %q, want %q", r.Fields["points"], want)
		}
	})

	t.Run("zero speed is reported", func(t *testing.T) {
		q := Query{Category: Walk, From: from, To: to, Speed: 0, StartSpeed: 0, Steps: repeatStep(time.Second, 1)}
		if r := Evaluate(e, f, q); r.Fields["error"] == "" {
			t.Fatalf("fields = %v, want an error", r.Fields)
		}
	})
}

func repeatStep(d time.Duration, n int) []time.Duration {
	steps := make([]time.Duration, n)
	for i := range steps {
		steps[i] = d
	}
	return steps
}

// wallEngine is openLoadedEngine with every move east out of the cells at
// index 2 of a row or column closed, so a walk east along the first row
// stops 30 units from its start.
func wallEngine(t *testing.T) (*engine.Engine, *pathfind.Finder) {
	t.Helper()

	var cells [block.CellCount]block.Cell
	for i := range cells {
		nswe := block.AllDirections
		if i%block.CellsX == 2 || i/block.CellsX == 2 {
			nswe &^= block.East
		}
		cells[i] = block.Cell{NSWE: nswe}
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
