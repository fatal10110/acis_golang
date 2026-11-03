package probe

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// TestCacheMatchesLegacyDecoderOnRealGeodata is the equivalence oracle of
// the compact layout over the real geodata: every region file is packed
// into a cache, mapped, and compared byte for byte with the heap loader,
// then every cell and layer of every block is compared with the previous
// decoder, layer handles included. It needs the datapack geodata, which the
// repository and CI do not have, so it runs locally or on the server (it
// skips otherwise); a -race build checks a sample of regions.
func TestCacheMatchesLegacyDecoderOnRealGeodata(t *testing.T) {
	dir := datapack.Geodata(t)
	files, err := filepath.Glob(filepath.Join(dir, "*_conv.dat"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no L2OFF region files in %s (%v)", dir, err)
	}
	var regions []Region
	for _, f := range files {
		var r Region
		if _, err := fmt.Sscanf(filepath.Base(f), "%d_%d_conv.dat", &r.X, &r.Y); err != nil {
			t.Fatalf("region file name %s: %v", f, err)
		}
		regions = append(regions, r)
	}
	if raceDetector {
		regions = []Region{regions[0], regions[len(regions)/3], regions[2*len(regions)/3], regions[len(regions)-1]}
	}

	cachePath := filepath.Join(t.TempDir(), "geodata.cache")
	stats, err := WriteCache(cachePath, dir, L2OFF, regions, zerolog.Nop())
	if err != nil {
		t.Fatalf("WriteCache(): %v", err)
	}
	t.Logf("%d regions: %+v, %d stored bytes", len(regions), stats, stats.IndexBytes+stats.ComplexBytes+stats.MultilayerBytes)
	mapped, err := openCache(cachePath, L2OFF, mustStat(t, dir, regions))
	if err != nil {
		t.Fatalf("openCache(): %v", err)
	}

	for i, r := range regions {
		t.Run(fmt.Sprintf("%d_%d", r.X, r.Y), func(t *testing.T) {
			t.Parallel()
			path := regionPath(dir, L2OFF, r.X, r.Y)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			legacy, err := decodeLegacyL2OFF(data)
			if err != nil {
				t.Fatal(err)
			}
			heap, _, err := readRegion(path, L2OFF)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(storedBytes(t, mapped[i]), storedBytes(t, heap)) {
				t.Fatal("mapped region differs from the heap loader's")
			}
			compareRegion(t, legacy, mapped[i])
		})
	}
}

// compareRegion compares every cell of every block. Flat and null cells
// compare their layers and nearest height at every cell and the full query
// set at one cell, since neither layout reads the cell coordinates.
func compareRegion(t *testing.T, want *legacyRegion, got *block.Region) {
	t.Helper()
	var wantCells, gotCells []block.Cell
	for bx := range block.RegionBlocksX {
		for by := range block.RegionBlocksY {
			kind := want.KindAt(bx, by)
			if got.KindAt(bx, by) != kind || got.HasGeodata(bx, by) != (kind != block.KindNull) {
				t.Fatalf("block %d_%d kind = %v, want %v", bx, by, got.KindAt(bx, by), kind)
			}
			if c := got.Cells(bx, by, 3, 5); !slices.Equal(c, want.AppendCells(nil, bx, by, 3, 5)) {
				t.Fatalf("block %d_%d Cells = %v", bx, by, c)
			}
			packed := kind == block.KindComplex || kind == block.KindMultilayer
			for cx := range block.CellsX {
				for cy := range block.CellsY {
					wantCells = want.AppendCells(wantCells[:0], bx, by, cx, cy)
					gotCells = got.AppendCells(gotCells[:0], bx, by, cx, cy)
					if !slices.Equal(gotCells, wantCells) || got.Layers(bx, by, cx, cy) != want.Layers(bx, by, cx, cy) {
						t.Fatalf("block %d_%d cell %d,%d = %v (%d layers), want %v", bx, by, cx, cy, gotCells, got.Layers(bx, by, cx, cy), wantCells)
					}
					if !packed && (cx != 0 || cy != 0) {
						if z := int32(wantCells[0].Height) + 1; got.HeightNearest(bx, by, cx, cy, z) != want.HeightNearest(bx, by, cx, cy, z) {
							t.Fatalf("block %d_%d cell %d,%d nearest height differs", bx, by, cx, cy)
						}
						continue
					}
					compareQueries(t, want, got, bx, by, cx, cy, wantCells)
				}
			}
		}
	}
}

func compareQueries(t *testing.T, want *legacyRegion, got *block.Region, bx, by, cx, cy int, cells []block.Cell) {
	t.Helper()
	zs := []int32{-40000, 40000}
	for i, c := range cells {
		h := int32(c.Height)
		zs = append(zs, h-1, h, h+1)
		if i > 0 {
			zs = append(zs, (h+int32(cells[i-1].Height))/2)
		}
	}
	cellQueries := [2]func(bx, by, cx, cy int, z int32) (int, block.Cell){got.AboveCell, got.BelowCell}
	for _, z := range zs {
		g := [3]int{got.Nearest(bx, by, cx, cy, z), got.Above(bx, by, cx, cy, z), got.Below(bx, by, cx, cy, z)}
		w := [3]int{want.Nearest(bx, by, cx, cy, z), want.Above(bx, by, cx, cy, z), want.Below(bx, by, cx, cy, z)}
		if g != w {
			t.Fatalf("block %d_%d cell %d,%d z %d: nearest/above/below handles = %v, want %v", bx, by, cx, cy, z, g, w)
		}
		for _, h := range g {
			if h >= 0 && (got.Height(bx, by, h) != want.Height(bx, by, h) || got.NSWE(bx, by, h) != want.NSWE(bx, by, h)) {
				t.Fatalf("block %d_%d handle %d resolves differently", bx, by, h)
			}
		}
		for i, query := range cellQueries {
			h, cell := query(bx, by, cx, cy, z)
			if h != w[i+1] {
				t.Fatalf("block %d_%d cell %d,%d z %d: %s handle = %d, want %d", bx, by, cx, cy, z, [2]string{"AboveCell", "BelowCell"}[i], h, w[i+1])
			}
			if h >= 0 && cell != (block.Cell{Height: want.Height(bx, by, h), NSWE: want.NSWE(bx, by, h)}) {
				t.Fatalf("block %d_%d cell %d,%d z %d: %s cell = %+v, want height %d NSWE %v", bx, by, cx, cy, z, [2]string{"AboveCell", "BelowCell"}[i], cell, want.Height(bx, by, h), want.NSWE(bx, by, h))
			}
		}
		if got.HeightNearest(bx, by, cx, cy, z) != want.HeightNearest(bx, by, cx, cy, z) || got.NSWENearest(bx, by, cx, cy, z) != want.NSWENearest(bx, by, cx, cy, z) {
			t.Fatalf("block %d_%d cell %d,%d z %d: nearest height/NSWE differ", bx, by, cx, cy, z)
		}
	}
}
