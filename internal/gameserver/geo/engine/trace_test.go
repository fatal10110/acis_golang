package engine

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
)

// walledEngine is open ground at height 0 with a wall on the east edge of
// geodata column 2 and a 40 high pillar at cell (5, 5).
func walledEngine(t testing.TB) *Engine {
	return newTestEngine(t, complexBlock(func(x, y int) block.Cell {
		switch {
		case x == 2:
			return block.Cell{Height: 0, NSWE: block.AllDirections &^ block.East}
		case x == 5 && y == 5:
			return block.Cell{Height: 40, NSWE: block.AllDirections}
		}
		return block.Cell{Height: 0, NSWE: block.AllDirections}
	}))
}

func TestTracedQueriesMatchUntraced(t *testing.T) {
	e := walledEngine(t)
	for ox := range 8 {
		for oy := range 8 {
			for tx := range 8 {
				for ty := range 8 {
					wox, woy, wtx, wty := worldX(ox)+3, worldY(oy)-5, worldX(tx)-2, worldY(ty)+6
					tr := &Trace{}
					if got, want := e.CanMoveTraced(wox, woy, 0, wtx, wty, 0, tr), e.CanMove(wox, woy, 0, wtx, wty, 0); got != want {
						t.Fatalf("CanMoveTraced(%d,%d -> %d,%d) = %v, want %v", ox, oy, tx, ty, got, want)
					}
					if got, want := e.ValidLocationTraced(wox, woy, 0, wtx, wty, 0, tr), e.ValidLocation(wox, woy, 0, wtx, wty, 0); got != want {
						t.Fatalf("ValidLocationTraced(%d,%d -> %d,%d) = %v, want %v", ox, oy, tx, ty, got, want)
					}
					if got, want := e.ValidFlyLocationTraced(wox, woy, 0, 40, wtx, wty, 30, tr), e.ValidFlyLocation(wox, woy, 0, 40, wtx, wty, 30); got != want {
						t.Fatalf("ValidFlyLocationTraced(%d,%d -> %d,%d) = %v, want %v", ox, oy, tx, ty, got, want)
					}
					oneWay := e.CanSeeOneWayTraced(wox, woy, 0, 30, wtx, wty, 0, 30, nil, tr) && e.CanSeeOneWayTraced(wtx, wty, 0, 30, wox, woy, 0, 30, nil, tr)
					if want := e.CanSeeWithHeights(wox, woy, 0, 30, wtx, wty, 0, 30); oneWay != want {
						t.Fatalf("CanSeeOneWayTraced both ways (%d,%d -> %d,%d) = %v, want %v", ox, oy, tx, ty, oneWay, want)
					}
				}
			}
		}
	}
}

func TestCanMoveTraceOutlinesWalkedCells(t *testing.T) {
	e := walledEngine(t)
	tr := &Trace{}
	// From cell 0 east to cell 4: the walk crosses into 1 and 2, then the
	// wall on 2's east edge stops it.
	ox, oy, tx, ty := worldX(0), worldY(1), worldX(4), worldY(1)
	if e.CanMoveTraced(ox, oy, 0, tx, ty, 0, tr) {
		t.Fatal("CanMoveTraced through the wall = true")
	}
	square := func(color uint32, x, y, z int) []Stroke {
		line := func(x, y, x2, y2 int) Stroke {
			return Stroke{Color: color, Line: true, X: x, Y: y, Z: z, X2: x2, Y2: y2, Z2: z}
		}
		return []Stroke{line(x, y, x, y+15), line(x+15, y+15, x, y+15), line(x+15, y+15, x+15, y), line(x, y, x+15, y)}
	}
	cell := func(geoX int) int { return worldX(geoX) - 8 }
	gy := worldY(1) - 8
	var want []Stroke
	want = append(want, square(TraceBlue, cell(0), gy, 1)...)
	want = append(want, square(TraceBlue, cell(4), gy, 0)...)
	for x := range 3 {
		want = append(want, Stroke{Label: "E", Color: TraceCyan, LabelColored: true, X: cell(x) + 15, Y: oy, Z: 0})
		want = append(want, square(TraceGreen, cell(x), gy, 0)...)
	}
	want = append(want, square(TraceRed, cell(3), gy, 0)...)
	if len(tr.Strokes) != len(want) {
		t.Fatalf("trace has %d strokes, want %d: %+v", len(tr.Strokes), len(want), tr.Strokes)
	}
	for i := range want {
		if tr.Strokes[i] != want[i] {
			t.Fatalf("stroke %d = %+v, want %+v", i, tr.Strokes[i], want[i])
		}
	}
}

func TestCanSeeTraceMarksWhereSightIsLost(t *testing.T) {
	e := walledEngine(t)
	tr := &Trace{}
	// Due north-east through the pillar at (5, 5), at ground height.
	if e.CanSeeOneWayTraced(worldX(3), worldY(3), 0, 0, worldX(7), worldY(7), 0, 0, nil, tr) {
		t.Fatal("CanSeeOneWayTraced through the pillar = true")
	}
	last := tr.Strokes[len(tr.Strokes)-1]
	if !last.Line || last.Color != TraceRed || last.Z != 40 {
		t.Fatalf("last stroke = %+v, want a red pillar cell outline at 40", last)
	}
	lost := tr.Strokes[len(tr.Strokes)-5]
	if lost.Line || lost.Color != TraceRed {
		t.Fatalf("stroke before the red outline = %+v, want the red point sight is lost at", lost)
	}
}

func TestNilTraceDoesNotAllocate(t *testing.T) {
	e := walledEngine(t)
	allocs := testing.AllocsPerRun(100, func() {
		_ = e.CanMoveTraced(worldX(0), worldY(1), 0, worldX(4), worldY(1), 0, nil)
		_ = e.ValidLocationTraced(worldX(0), worldY(1), 0, worldX(4), worldY(1), 0, nil)
		_ = e.CanSeeOneWayTraced(worldX(3), worldY(3), 0, 0, worldX(7), worldY(7), 0, 0, nil, nil)
	})
	if allocs != 0 {
		t.Fatalf("untraced query allocations = %.0f, want 0", allocs)
	}
}
