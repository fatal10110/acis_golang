package block

import "testing"

func TestComplex(t *testing.T) {
	var cells [CellCount]Cell
	// Give every cell a distinct height/NSWE so a wrong index formula shows up.
	for x := 0; x < CellsX; x++ {
		for y := 0; y < CellsY; y++ {
			i := cellIndex(x, y)
			cells[i] = Cell{Height: int16(i * 8), NSWE: NSWE(i % 16)}
		}
	}
	b := NewComplex(cells)

	if got := b.Kind(); got != KindComplex {
		t.Errorf("Kind() = %v, want %v", got, KindComplex)
	}
	if !b.HasGeodata() {
		t.Errorf("HasGeodata() = false, want true")
	}

	cellsToCheck := [][2]int{{0, 0}, {2, 3}, {7, 7}, {5, 0}}
	for _, xy := range cellsToCheck {
		x, y := xy[0], xy[1]
		want := cells[cellIndex(x, y)]

		if got := b.Layers(x, y); got != 1 {
			t.Errorf("Layers(%d,%d) = %d, want 1", x, y, got)
		}
		if got := b.HeightNearest(x, y, 0); got != want.Height {
			t.Errorf("HeightNearest(%d,%d,0) = %d, want %d", x, y, got, want.Height)
		}
		if got := b.NSWENearest(x, y, 0); got != want.NSWE {
			t.Errorf("NSWENearest(%d,%d,0) = %v, want %v", x, y, got, want.NSWE)
		}

		handle := b.Nearest(x, y, 12345) // worldZ must be ignored: single layer per cell.
		if got := b.Height(handle); got != want.Height {
			t.Errorf("Height(Nearest(%d,%d,...)) = %d, want %d", x, y, got, want.Height)
		}
		if got := b.NSWE(handle); got != want.NSWE {
			t.Errorf("NSWE(Nearest(%d,%d,...)) = %v, want %v", x, y, got, want.NSWE)
		}
	}

	// Above/Below gate strictly on this cell's single height.
	x, y := 4, 4
	h := cells[cellIndex(x, y)].Height
	if got := b.Above(x, y, int32(h)-1); got != cellIndex(x, y) {
		t.Errorf("Above below height: got %d, want %d", got, cellIndex(x, y))
	}
	if got := b.Above(x, y, int32(h)); got != -1 {
		t.Errorf("Above at height: got %d, want -1", got)
	}
	if got := b.Below(x, y, int32(h)+1); got != cellIndex(x, y) {
		t.Errorf("Below above height: got %d, want %d", got, cellIndex(x, y))
	}
	if got := b.Below(x, y, int32(h)); got != -1 {
		t.Errorf("Below at height: got %d, want -1", got)
	}
}
