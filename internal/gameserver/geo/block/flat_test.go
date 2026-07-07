package block

import "testing"

func TestFlat(t *testing.T) {
	b := NewFlat(80)

	if got := b.Kind(); got != KindFlat {
		t.Errorf("Kind() = %v, want %v", got, KindFlat)
	}
	if !b.HasGeodata() {
		t.Errorf("HasGeodata() = false, want true")
	}

	// Height/NSWE/layer count must be identical at every cell coordinate,
	// since a Flat block has no per-cell variation.
	for _, cell := range [][2]int{{0, 0}, {3, 5}, {CellsX - 1, CellsY - 1}} {
		x, y := cell[0], cell[1]
		if got := b.Layers(x, y); got != 1 {
			t.Errorf("Layers(%d,%d) = %d, want 1", x, y, got)
		}
		if got := b.HeightNearest(x, y, 0); got != 80 {
			t.Errorf("HeightNearest(%d,%d,0) = %d, want 80", x, y, got)
		}
		if got := b.NSWENearest(x, y, 0); got != AllDirections {
			t.Errorf("NSWENearest(%d,%d,0) = %v, want all", x, y, got)
		}
	}

	cases := []struct {
		name      string
		worldZ    int32
		wantAbove int
		wantBelow int
	}{
		{"below block height", 50, 0, -1},
		{"above block height", 100, -1, 0},
		{"equal to block height", 80, -1, -1},
	}
	for _, c := range cases {
		if got := b.Above(0, 0, c.worldZ); got != c.wantAbove {
			t.Errorf("%s: Above(0,0,%d) = %d, want %d", c.name, c.worldZ, got, c.wantAbove)
		}
		if got := b.Below(0, 0, c.worldZ); got != c.wantBelow {
			t.Errorf("%s: Below(0,0,%d) = %d, want %d", c.name, c.worldZ, got, c.wantBelow)
		}
	}

	// Nearest, Height, and NSWE ignore their handle/coordinates entirely.
	if got := b.Nearest(2, 2, 999); got != 0 {
		t.Errorf("Nearest(...) = %d, want 0", got)
	}
	if got := b.Height(0); got != 80 {
		t.Errorf("Height(0) = %d, want 80", got)
	}
	if got := b.NSWE(0); got != AllDirections {
		t.Errorf("NSWE(0) = %v, want all", got)
	}
}
