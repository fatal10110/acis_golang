package geodata

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// TestNearestMatchesFileOrderScanAtMidpoints probes every multilayer cell
// of the shipped geodata at the exact midpoint between each pair of
// adjacent layers, the one height where two layers are equally close, and
// compares the loaded region's nearest layer with the reference scan run
// over the region file's own layer order (top-down): keep walking while the
// distance does not grow, answer the last layer reached. A tie therefore
// resolves to the lower layer, and an out-of-order cell to the first local
// minimum. It needs the datapack geodata, which the repository and CI do
// not have, so it runs locally (it skips otherwise); a -race build checks a
// sample of regions.
func TestNearestMatchesFileOrderScanAtMidpoints(t *testing.T) {
	dir := datapack.Geodata(t)
	files, err := filepath.Glob(filepath.Join(dir, "*_conv.dat"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no L2OFF region files in %s (%v)", dir, err)
	}
	if raceDetector {
		files = []string{files[0], files[len(files)/3], files[2*len(files)/3], files[len(files)-1]}
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			legacy, err := decodeLegacyL2OFF(data)
			if err != nil {
				t.Fatal(err)
			}
			region, _, err := readRegion(path, L2OFF)
			if err != nil {
				t.Fatal(err)
			}
			probes := 0
			for bx := range block.RegionBlocksX {
				for by := range block.RegionBlocksY {
					if legacy.KindAt(bx, by) != block.KindMultilayer {
						continue
					}
					e := legacy.entry(bx, by)
					for cx := range block.CellsX {
						for cy := range block.CellsY {
							ci := legacyCellIndex(cx, cy)
							count := legacy.Layers(bx, by, cx, cy)
							// The legacy decoder stores a cell's layers in
							// reverse file order.
							file := make([]int32, count)
							for j := range count {
								file[j] = int32(block.DecodeCell(legacy.multilayerCodeAt(e, ci, count-1-j)).Height)
							}
							for j := 1; j < count; j++ {
								z := (file[j-1] + file[j]) / 2
								want := fileOrderNearest(file, z)
								if got := int32(region.HeightNearest(bx, by, cx, cy, z)); got != want {
									t.Fatalf("block %d_%d cell %d,%d layers %v z %d: HeightNearest = %d, want %d", bx, by, cx, cy, file, z, got, want)
								}
								probes++
							}
						}
					}
				}
			}
			t.Logf("%d midpoint probes", probes)
		})
	}
}

// fileOrderNearest is the reference nearest-layer scan over layers in file
// order: it stops at the first layer farther from z than the one before.
func fileOrderNearest(layers []int32, z int32) int32 {
	best := layers[0]
	limit := int32(1<<31 - 1)
	for _, h := range layers {
		d := h - z
		if d < 0 {
			d = -d
		}
		if d > limit {
			break
		}
		limit, best = d, h
	}
	return best
}
