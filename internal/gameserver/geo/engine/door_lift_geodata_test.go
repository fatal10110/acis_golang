package engine_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/dynamic"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/reader"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// regionGeoCells is the width of one region in geodata cells.
const regionGeoCells = block.RegionBlocksX * block.CellsX

// shippedDoors loads doors.xml from the datapack, which the repository and
// CI do not have.
func shippedDoors(t *testing.T) *door.Table {
	t.Helper()
	doors, err := xml.LoadDoors(datapack.Path(t, "data", "xml", "doors.xml"), zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadDoors: %v", err)
	}
	return doors
}

// doorTile returns the region tile holding the door's position.
func doorTile(tmpl *door.Template) [2]int {
	return [2]int{
		engine.TileXMin + (tmpl.Position.X-engine.WorldXMin)/engine.TileSize,
		engine.TileYMin + (tmpl.Position.Y-engine.WorldYMin)/engine.TileSize,
	}
}

// readRegion reads one shipped region, or returns nil when the datapack
// ships no geodata for the tile.
func readRegion(t *testing.T, tile [2]int) *block.Region {
	t.Helper()
	path := filepath.Join(datapack.Geodata(t), fmt.Sprintf("%d_%d_conv.dat", tile[0], tile[1]))
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	region, _, err := reader.ReadL2OFF(path)
	if err != nil {
		t.Fatalf("ReadL2OFF(%s): %v", path, err)
	}
	return region
}

// closeDoorOn closes the door on its own engine holding only region, and
// returns the engine, the door's geodata object and the region's first
// geodata cell.
func closeDoorOn(t *testing.T, tmpl *door.Template, tile [2]int, region *block.Region) (*engine.Engine, dynamic.Object, [2]int) {
	t.Helper()
	e := engine.New()
	if err := e.SetRegion(tile[0], tile[1], region); err != nil {
		t.Fatalf("SetRegion: %v", err)
	}
	shape, err := dynamic.NewDoorObject(tmpl, e)
	if err != nil {
		t.Fatalf("door %d: NewDoorObject: %v", tmpl.ID, err)
	}
	e.AddObject(shape)
	return e, shape, [2]int{(tile[0] - engine.TileXMin) * regionGeoCells, (tile[1] - engine.TileYMin) * regionGeoCells}
}

// checkDoorLift checks every blocked cell of a closed door that sits on a
// single-layer cell of a multilayer block: the lift stops a cell's ignore
// height short of the cell's own layer when that layer is above the door's
// foot and not above its top, and reaches the door's top otherwise. It
// returns how many cells took the short lift.
func checkDoorLift(t *testing.T, id int, e *engine.Engine, region *block.Region, shape dynamic.Object, origin [2]int) int {
	t.Helper()
	minOZ, maxOZ := shape.GeoZ(), shape.GeoZ()+shape.Height()
	data := shape.GeoData()
	capped := 0
	for i := range data {
		for j := range data[i] {
			if data[i][j] != block.NoDirections {
				continue
			}
			gx, gy := shape.GeoX()+i, shape.GeoY()+j
			lx, ly := gx-origin[0], gy-origin[1]
			if lx < 0 || ly < 0 || lx >= regionGeoCells || ly >= regionGeoCells {
				continue
			}
			bx, by, cx, cy := lx/block.CellsX, ly/block.CellsY, lx%block.CellsX, ly%block.CellsY
			if region.KindAt(bx, by) != block.KindMultilayer || region.Layers(bx, by, cx, cy) != 1 {
				continue
			}
			h := int(region.HeightNearest(bx, by, cx, cy, 0))
			want := maxOZ
			if h > minOZ && h <= maxOZ {
				want = h - block.CellIgnoreHeight
				capped++
			}
			if got := e.HeightNearest(gx, gy, minOZ); int(got) != want {
				t.Errorf("door %d cell %d,%d (layer %d, door %d..%d): height = %d, want %d", id, gx, gy, h, minOZ, maxOZ, got, want)
			}
		}
	}
	return capped
}

// TestClosedDoorLiftOnSingleLayerCellsOfMultilayerBlocks pins the height a
// closed door gives the single-layer cells it fills on a multilayer block.
// One cell of each door carries the height the reference server reports
// for it.
func TestClosedDoorLiftOnSingleLayerCellsOfMultilayerBlocks(t *testing.T) {
	doors := shippedDoors(t)
	for _, tc := range []struct {
		door int
		cell [2]int // geodata cell
		want int16
	}{
		{20200001, [2]int{8966, 22225}, -3464},
		{24160021, [2]int{17550, 13407}, -2424},
	} {
		tmpl, ok := doors.Get(tc.door)
		if !ok {
			t.Fatalf("door %d missing", tc.door)
		}
		tile := doorTile(tmpl)
		region := readRegion(t, tile)
		if region == nil {
			t.Fatalf("door %d: no geodata for region %d_%d", tc.door, tile[0], tile[1])
		}
		e, shape, origin := closeDoorOn(t, tmpl, tile, region)
		if got := e.HeightNearest(tc.cell[0], tc.cell[1], shape.GeoZ()); got != tc.want {
			t.Errorf("door %d cell %d,%d: height = %d, want %d", tc.door, tc.cell[0], tc.cell[1], got, tc.want)
		}
		if checkDoorLift(t, tc.door, e, region, shape, origin) == 0 {
			t.Errorf("door %d fills no single-layer multilayer cell under its top", tc.door)
		}
	}
}

// TestClosedDoorLiftOnEveryShippedDoor runs the same lift rule over every
// door of doors.xml whose region the datapack ships, each closed alone on
// its region.
func TestClosedDoorLiftOnEveryShippedDoor(t *testing.T) {
	regions := map[[2]int]*block.Region{}
	checked, cappedDoors, capped := 0, 0, 0
	for _, tmpl := range shippedDoors(t).All() {
		tile := doorTile(tmpl)
		region, seen := regions[tile]
		if !seen {
			region = readRegion(t, tile)
			regions[tile] = region
		}
		if region == nil {
			continue
		}
		e, shape, origin := closeDoorOn(t, tmpl, tile, region)
		n := checkDoorLift(t, tmpl.ID, e, region, shape, origin)
		checked++
		if n > 0 {
			cappedDoors++
			capped += n
		}
	}
	t.Logf("doors checked: %d; short lifts: %d cells over %d doors", checked, capped, cappedDoors)
	// The reference sweep over the same datapack finds 96 such cells under
	// 14 doors.
	if capped != 96 || cappedDoors != 14 {
		t.Errorf("short lifts: %d cells over %d doors, want 96 over 14", capped, cappedDoors)
	}
}
