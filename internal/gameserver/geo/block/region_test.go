package block

import "testing"

func TestRegionQueriesPackedBlocks(t *testing.T) {
	r := NewRegion()

	r.SetFlat(0, -32)
	if got := r.KindAt(0, 0); got != KindFlat {
		t.Fatalf("flat kind = %v, want flat", got)
	}
	if got := r.HeightNearest(0, 0, 7, 7, 0); got != -32 {
		t.Fatalf("flat height = %d, want -32", got)
	}
	if got := r.NSWENearest(0, 0, 7, 7, 0); got != AllDirections {
		t.Fatalf("flat nswe = %v, want all", got)
	}

	var complex [CellCount]Cell
	complex[cellIndex(2, 3)] = Cell{Height: 64, NSWE: North | East}
	if err := r.SetComplex(1, complex); err != nil {
		t.Fatalf("SetComplex: %v", err)
	}
	if got := r.KindAt(0, 1); got != KindComplex {
		t.Fatalf("complex kind = %v, want complex", got)
	}
	if got := r.HeightNearest(0, 1, 2, 3, 0); got != 64 {
		t.Fatalf("complex height = %d, want 64", got)
	}
	if got := r.NSWENearest(0, 1, 2, 3, 0); got != North|East {
		t.Fatalf("complex nswe = %v, want NE", got)
	}

	var multi [CellCount][]Cell
	for i := range multi {
		multi[i] = []Cell{{Height: 0, NSWE: AllDirections}}
	}
	multi[cellIndex(4, 5)] = []Cell{
		{Height: -16, NSWE: West},
		{Height: 48, NSWE: South | East},
	}
	if err := r.SetMultilayer(2, multi); err != nil {
		t.Fatalf("SetMultilayer: %v", err)
	}
	if got := r.KindAt(0, 2); got != KindMultilayer {
		t.Fatalf("multilayer kind = %v, want multilayer", got)
	}
	if got := r.Layers(0, 2, 4, 5); got != 2 {
		t.Fatalf("multilayer layers = %d, want 2", got)
	}
	if got := r.HeightNearest(0, 2, 4, 5, 40); got != 48 {
		t.Fatalf("multilayer nearest height = %d, want 48", got)
	}
	if got := r.NSWENearest(0, 2, 4, 5, 40); got != South|East {
		t.Fatalf("multilayer nearest nswe = %v, want SE", got)
	}

	if got := r.KindAt(0, 3); got != KindNull {
		t.Fatalf("unset kind = %v, want null", got)
	}
	if got := r.HeightNearest(0, 3, 0, 0, 123); got != 123 {
		t.Fatalf("unset height = %d, want queried Z", got)
	}
}

func TestNewRegionFromBlocks(t *testing.T) {
	blocks := make([]Block, RegionBlockCount)
	blocks[0] = NewFlat(80)

	r, err := NewRegionFromBlocks(blocks)
	if err != nil {
		t.Fatalf("NewRegionFromBlocks: %v", err)
	}
	if got := r.HeightNearest(0, 0, 0, 0, 0); got != 80 {
		t.Fatalf("height = %d, want 80", got)
	}
	if got := r.KindAt(0, 1); got != KindNull {
		t.Fatalf("nil block kind = %v, want null", got)
	}
}

func TestRegionRejectsPackedDataOverflow(t *testing.T) {
	r := NewRegion()

	if _, err := r.appendData(regionValueMask + 1); err == nil {
		t.Fatal("appendData overflow error = nil, want error")
	}
}
