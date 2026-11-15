package engine_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/dynamic"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/reader"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// regionGeoCells is the width of one region in geodata cells.
const regionGeoCells = block.RegionBlocksX * block.CellsX

// closedDoorOnGeodata loads the shipped region under door id, closes the
// door on it, and returns the engine, the region's static data, the door's
// geodata object and the region's first geodata cell. It needs the
// datapack, which the repository and CI do not have.
func closedDoorOnGeodata(t *testing.T, id int) (*engine.Engine, *block.Region, dynamic.Object, [2]int) {
	t.Helper()
	doors, err := xml.LoadDoors(datapack.Path(t, "data", "xml", "doors.xml"), zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadDoors: %v", err)
	}
	tmpl, ok := doors.Get(id)
	if !ok {
		t.Fatalf("door %d missing", id)
	}
	tileX := engine.TileXMin + (tmpl.Position.X-engine.WorldXMin)/engine.TileSize
	tileY := engine.TileYMin + (tmpl.Position.Y-engine.WorldYMin)/engine.TileSize
	path := filepath.Join(datapack.Geodata(t), fmt.Sprintf("%d_%d_conv.dat", tileX, tileY))
	region, _, err := reader.ReadL2OFF(path)
	if err != nil {
		t.Fatalf("ReadL2OFF(%s): %v", path, err)
	}
	e := engine.New()
	if err := e.SetRegion(tileX, tileY, region); err != nil {
		t.Fatalf("SetRegion: %v", err)
	}
	shape, err := dynamic.NewDoorObject(tmpl, e)
	if err != nil {
		t.Fatalf("NewDoorObject: %v", err)
	}
	e.AddObject(shape)
	return e, region, shape, [2]int{(tileX - engine.TileXMin) * regionGeoCells, (tileY - engine.TileYMin) * regionGeoCells}
}

// TestClosedDoorLiftOnSingleLayerCellsOfMultilayerBlocks pins the height a
// closed door gives the single-layer cells it fills on a multilayer block:
// the lift stops a cell's ignore height short of the cell's own layer when
// that layer is above the door's foot and not above its top, and reaches
// the door's top otherwise. One cell of each door carries the height the
// reference server reports for it.
func TestClosedDoorLiftOnSingleLayerCellsOfMultilayerBlocks(t *testing.T) {
	for _, tc := range []struct {
		door int
		cell [2]int // geodata cell
		want int16
	}{
		{20200001, [2]int{8966, 22225}, -3464},
		{24160021, [2]int{17550, 13407}, -2424},
	} {
		e, region, shape, origin := closedDoorOnGeodata(t, tc.door)
		minOZ, maxOZ := shape.GeoZ(), shape.GeoZ()+shape.Height()
		if got := e.HeightNearest(tc.cell[0], tc.cell[1], minOZ); got != tc.want {
			t.Errorf("door %d cell %d,%d: height = %d, want %d", tc.door, tc.cell[0], tc.cell[1], got, tc.want)
		}
		data := shape.GeoData()
		capped := 0
		for i := range data {
			for j := range data[i] {
				if data[i][j] != block.NoDirections {
					continue
				}
				gx, gy := shape.GeoX()+i, shape.GeoY()+j
				lx, ly := gx-origin[0], gy-origin[1]
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
					t.Errorf("door %d cell %d,%d (layer %d, door %d..%d): height = %d, want %d", tc.door, gx, gy, h, minOZ, maxOZ, got, want)
				}
			}
		}
		if capped == 0 {
			t.Errorf("door %d fills no single-layer multilayer cell under its top", tc.door)
		}
	}
}
