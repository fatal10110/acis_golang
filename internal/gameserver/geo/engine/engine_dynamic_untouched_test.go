package engine

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
)

// TestEngineDynamicBlockUntouchedCellsReadAsBase pins the reads of a block
// carrying a closed door on cells the door does not cover: they answer as
// the static geodata does, so walking, sight and the height a walk lands
// on are unchanged beside the door, and so is flying past it, while the
// door itself still blocks.
func TestEngineDynamicBlockUntouchedCellsReadAsBase(t *testing.T) {
	for _, tc := range []struct {
		name string
		base block.Block
	}{
		{"flat", block.NewFlat(32)},
		{"complex", complexBlock(func(int, int) block.Cell { return block.Cell{Height: 32, NSWE: block.AllDirections} })},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := New()
			region, err := block.NewRegionFromBlocks([]block.Block{tc.base})
			if err != nil {
				t.Fatalf("NewRegionFromBlocks: %v", err)
			}
			if err := e.SetRegion(TileXMin, TileYMin, region); err != nil {
				t.Fatalf("SetRegion: %v", err)
			}
			// The door covers cell (6, 6) only; the walk runs along cell
			// row 0 of the same block, and the blocked walk crosses the door.
			e.AddObject(&dynamicStub{x: 6, y: 6, z: 32, height: 32, data: [][]block.NSWE{{block.NoDirections}}})
			if dynamicBlockCount(e) != 1 {
				t.Fatal("the door did not overlay its block")
			}

			ox, oy, tx := worldX(0), worldY(0), worldX(5)
			if !e.CanMove(ox, oy, 32, tx, oy, 32) {
				t.Error("CanMove() beside the door = false, want true")
			}
			if !e.CanSee(ox, oy, 32, tx, oy, 32) {
				t.Error("CanSee() beside the door = false, want true")
			}
			if !e.CanFly(ox, oy, 100, 32, tx, oy, 100) {
				t.Error("CanFly() beside the door = false, want true")
			}
			if h, ok := e.Above(0, 0, 0); !ok || h != 32 {
				t.Errorf("Above() beside the door = %d, %v; want 32, true", h, ok)
			}
			if h, _, ok := e.NodeBelow(0, 0, 100); !ok || h != 32 {
				t.Errorf("NodeBelow() beside the door = %d, %v; want 32, true", h, ok)
			}
			if e.CanMove(worldX(4), worldY(6), 32, worldX(7), worldY(6), 32) {
				t.Error("CanMove() through the door = true, want false")
			}
		})
	}
}
