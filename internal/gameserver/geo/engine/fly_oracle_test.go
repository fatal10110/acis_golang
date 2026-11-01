package engine

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// flyOracleEngine builds the synthetic geodata testdata/oracle/FlyProbe.java
// builds (layout in testdata/oracle/README.md), block for block. Blocks are
// placed by grid block coordinates; every other block is null geodata.
func flyOracleEngine(t *testing.T) *Engine {
	t.Helper()
	const (
		gridBlocksX = (TileXMax - TileXMin + 1) * block.RegionBlocksX
		gridBlocksY = (TileYMax - TileYMin + 1) * block.RegionBlocksY
	)
	regions := map[[2]int]*block.Region{}
	place := func(blockX, blockY int, set func(r *block.Region, index int) error) {
		t.Helper()
		tile := [2]int{TileXMin + blockX/block.RegionBlocksX, TileYMin + blockY/block.RegionBlocksY}
		r := regions[tile]
		if r == nil {
			r = block.NewRegion()
			regions[tile] = r
		}
		index := (blockX%block.RegionBlocksX)*block.RegionBlocksY + blockY%block.RegionBlocksY
		if err := set(r, index); err != nil {
			t.Fatalf("block %d,%d: %v", blockX, blockY, err)
		}
	}
	flat := func(blockX, blockY int, height int16) {
		place(blockX, blockY, func(r *block.Region, index int) error {
			r.SetFlat(index, height)
			return nil
		})
	}
	complex := func(blockX, blockY int, height func(x, y int) int16) {
		var cells [block.CellCount]block.Cell
		for x := range block.CellsX {
			for y := range block.CellsY {
				cells[x*block.CellsY+y] = block.Cell{Height: height(x, y), NSWE: block.AllDirections}
			}
		}
		place(blockX, blockY, func(r *block.Region, index int) error { return r.SetComplex(index, cells) })
	}
	multilayer := func(blockX, blockY int, heights func(x, y int) []int16) {
		var cells [block.CellCount][]block.Cell
		for x := range block.CellsX {
			for y := range block.CellsY {
				for _, h := range heights(x, y) {
					cells[x*block.CellsY+y] = append(cells[x*block.CellsY+y], block.Cell{Height: h, NSWE: block.AllDirections})
				}
			}
		}
		place(blockX, blockY, func(r *block.Region, index int) error { return r.SetMultilayer(index, cells) })
	}

	complex(0, 0, func(x, _ int) int16 {
		if x >= 4 {
			return 200
		}
		return 0
	})
	flat(1, 0, 50)
	complex(2, 0, func(x, _ int) int16 {
		if x == 3 || x == 4 {
			return -200
		}
		return 0
	})
	complex(0, 2, func(x, y int) int16 { return int16(8 * (x + y)) })
	multilayer(1, 2, func(x, y int) []int16 {
		switch {
		case x == 7:
			return []int16{0, 80, 400}
		case (x+y)%2 == 0:
			return []int16{0, 152}
		}
		return []int16{0}
	})
	flat(gridBlocksX-1, 0, 30)
	flat(0, gridBlocksY-1, 30)
	flat(gridBlocksX-1, gridBlocksY-1, 30)

	e := New()
	for tile, r := range regions {
		if err := e.SetRegion(tile[0], tile[1], r); err != nil {
			t.Fatalf("SetRegion %v: %v", tile, err)
		}
	}
	return e
}

type flyGoldenLine struct {
	name   string
	from   location.Location
	height float64
	to     location.Location
	canFly bool
	stop   location.Location
}

// flyGolden reads testdata/fly.golden, the output of testdata/oracle/run.sh:
// "fly <name> <ox oy oz> <oheight> <tx ty tz> <canFly> <stop x y z>", tab
// separated.
func flyGolden(t *testing.T) []flyGoldenLine {
	t.Helper()
	f, err := os.Open("testdata/fly.golden")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	point := func(s string) location.Location {
		var p [3]int
		fields := strings.Fields(s)
		if len(fields) != 3 {
			t.Fatalf("golden point %q: want 3 coordinates", s)
		}
		for i, v := range fields {
			n, err := strconv.Atoi(v)
			if err != nil {
				t.Fatalf("golden point %q: %v", s, err)
			}
			p[i] = n
		}
		return location.Location{X: p[0], Y: p[1], Z: p[2]}
	}
	var out []flyGoldenLine
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Split(sc.Text(), "\t")
		if len(fields) != 7 || fields[0] != "fly" {
			t.Fatalf("golden line %q: want 7 tab separated fields starting with fly", sc.Text())
		}
		height, err := strconv.ParseFloat(fields[3], 64)
		if err != nil {
			t.Fatalf("golden line %q: %v", sc.Text(), err)
		}
		can, err := strconv.ParseBool(fields[5])
		if err != nil {
			t.Fatalf("golden line %q: %v", sc.Text(), err)
		}
		out = append(out, flyGoldenLine{
			name: fields[1], from: point(fields[2]), height: height, to: point(fields[4]),
			canFly: can, stop: point(fields[6]),
		})
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestFlyMatchesReferenceProbe replays every golden query: the reference's own
// fly queries on the synthetic geodata, one line per CanFly and
// ValidFlyLocation pair.
func TestFlyMatchesReferenceProbe(t *testing.T) {
	t.Parallel()
	e := flyOracleEngine(t)
	lines := flyGolden(t)
	if len(lines) == 0 {
		t.Fatal("golden has no fly lines")
	}
	var clear, blocked int
	for _, l := range lines {
		if l.canFly {
			clear++
		} else {
			blocked++
		}
		t.Run(l.name, func(t *testing.T) {
			if got := e.CanFly(l.from.X, l.from.Y, l.from.Z, l.height, l.to.X, l.to.Y, l.to.Z); got != l.canFly {
				t.Errorf("CanFly() = %v, want %v", got, l.canFly)
			}
			if got := e.ValidFlyLocation(l.from.X, l.from.Y, l.from.Z, l.height, l.to.X, l.to.Y, l.to.Z); got != l.stop {
				t.Errorf("ValidFlyLocation() = %+v, want %+v", got, l.stop)
			}
		})
	}
	if clear == 0 || blocked == 0 {
		t.Errorf("golden has %d clear and %d blocked lines, want both", clear, blocked)
	}
}
