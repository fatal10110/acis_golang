package block

import "testing"

func TestKindString(t *testing.T) {
	cases := []struct {
		k    Kind
		want string
	}{
		{KindNull, "null"},
		{KindFlat, "flat"},
		{KindComplex, "complex"},
		{KindMultilayer, "multilayer"},
		{Kind(99), "Kind(99)"},
	}
	for _, c := range cases {
		if got := c.k.String(); got != c.want {
			t.Errorf("Kind(%d).String() = %q, want %q", int(c.k), got, c.want)
		}
	}
}

// TestGeometryConstants pins the constants this package exists to get
// right, cross-checked directly against GeoStructure.java's values
// rather than trusted from memory.
func TestGeometryConstants(t *testing.T) {
	if CellSize != 16 {
		t.Errorf("CellSize = %d, want 16", CellSize)
	}
	if CellHeight != 8 {
		t.Errorf("CellHeight = %d, want 8", CellHeight)
	}
	if CellIgnoreHeight != 48 {
		t.Errorf("CellIgnoreHeight = %d, want 48", CellIgnoreHeight)
	}
	if CellsX != 8 || CellsY != 8 || CellCount != 64 {
		t.Errorf("CellsX/CellsY/CellCount = %d/%d/%d, want 8/8/64", CellsX, CellsY, CellCount)
	}
	if RegionBlocksX != 256 || RegionBlocksY != 256 || RegionBlockCount != 65536 {
		t.Errorf("RegionBlocksX/Y/Count = %d/%d/%d, want 256/256/65536", RegionBlocksX, RegionBlocksY, RegionBlockCount)
	}
	if MaxLayers != 127 {
		t.Errorf("MaxLayers = %d, want 127", MaxLayers)
	}
}

// TestBlockKindsSatisfyInterface confirms every concrete block type
// implements Block and reports its own Kind, so a caller holding a
// []Block can dispatch on Kind() alone.
func TestBlockKindsSatisfyInterface(t *testing.T) {
	multi, err := NewMultilayer(func() (cells [CellCount][]Cell) {
		for i := range cells {
			cells[i] = []Cell{{Height: 0, NSWE: AllDirections}}
		}
		return
	}())
	if err != nil {
		t.Fatalf("NewMultilayer: %v", err)
	}

	blocks := []Block{
		NewFlat(0),
		NewComplex([CellCount]Cell{}),
		multi,
		&Null{},
	}
	wantKinds := []Kind{KindFlat, KindComplex, KindMultilayer, KindNull}
	for i, b := range blocks {
		if got := b.Kind(); got != wantKinds[i] {
			t.Errorf("blocks[%d].Kind() = %v, want %v", i, got, wantKinds[i])
		}
	}
}
