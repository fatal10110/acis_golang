package world

import "testing"

// RegionsX/RegionsY are a known-good vector: the aCis Interlude world grid is
// 176 by 256 regions.
func TestGridDimensions(t *testing.T) {
	if RegionsX != 176 {
		t.Errorf("RegionsX = %d, want 176", RegionsX)
	}
	if RegionsY != 256 {
		t.Errorf("RegionsY = %d, want 256", RegionsY)
	}
}

func TestGrid_RegionAt(t *testing.T) {
	g := NewGrid()

	tests := []struct {
		name   string
		x, y   int
		wantOK bool
		wantTX int
		wantTY int
	}{
		{"min corner", MinX, MinY, true, 0, 0},
		{"max corner", MaxX, MaxY, true, RegionsX - 1, RegionsY - 1},
		{"one below min x", MinX - 1, MinY, false, 0, 0},
		{"one above max x", MaxX + 1, MinY, false, 0, 0},
		{"one below min y", MinX, MinY - 1, false, 0, 0},
		{"one above max y", MinX, MaxY + 1, false, 0, 0},
		{"second region boundary", MinX + regionSize, MinY, true, 1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, ok := g.RegionAt(tt.x, tt.y)
			if ok != tt.wantOK {
				t.Fatalf("RegionAt(%d, %d) ok = %v, want %v", tt.x, tt.y, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if r.tileX != tt.wantTX || r.tileY != tt.wantTY {
				t.Errorf("RegionAt(%d, %d) = tile (%d, %d), want (%d, %d)", tt.x, tt.y, r.tileX, r.tileY, tt.wantTX, tt.wantTY)
			}
			if r != g.regions[tt.wantTX][tt.wantTY] {
				t.Errorf("RegionAt(%d, %d) did not return the grid's own Region instance", tt.x, tt.y)
			}
		})
	}
}

func TestGrid_Neighbors(t *testing.T) {
	g := NewGrid()

	tests := []struct {
		name      string
		tileX     int
		tileY     int
		depth     int
		wantCount int
	}{
		{"corner depth 1", 0, 0, 1, 4},
		{"center depth 1", 10, 10, 1, 9},
		{"depth 0 is self only", 10, 10, 0, 1},
		{"edge depth 1", RegionsX - 1, 10, 1, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := g.regions[tt.tileX][tt.tileY]
			neighbors := g.Neighbors(r, tt.depth)
			if len(neighbors) != tt.wantCount {
				t.Errorf("Neighbors(tile %d,%d, depth %d) returned %d regions, want %d", tt.tileX, tt.tileY, tt.depth, len(neighbors), tt.wantCount)
			}

			found := false
			for _, n := range neighbors {
				if n == r {
					found = true
				}
			}
			if !found {
				t.Error("Neighbors did not include the region itself")
			}
		})
	}
}
