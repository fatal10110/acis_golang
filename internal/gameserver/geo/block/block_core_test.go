package block

import (
	"math"
	"testing"
)

// ---- from block_test.go ----
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

// ---- from complex_test.go ----
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

// ---- from flat_test.go ----
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

// ---- from multilayer_test.go ----
// newTestMultilayer builds a Multilayer block where cell (0,0) has the
// given ascending heights as its layers (NSWE set to a distinguishable,
// non-zero value per layer) and every other cell has a single dummy
// layer, satisfying NewMultilayer's 1..MaxLayers-per-cell requirement.
func newTestMultilayer(t *testing.T, heights []int16) *Multilayer {
	t.Helper()
	var cells [CellCount][]Cell
	for i := range cells {
		cells[i] = []Cell{{Height: 0, NSWE: AllDirections}}
	}
	layers := make([]Cell, len(heights))
	for i, h := range heights {
		layers[i] = Cell{Height: h, NSWE: NSWE((i % 15) + 1)}
	}
	cells[cellIndex(0, 0)] = layers

	b, err := NewMultilayer(cells)
	if err != nil {
		t.Fatalf("NewMultilayer: %v", err)
	}
	return b
}

// TestMultilayerNearest checks the tie-break behavior of Nearest against
// vectors generated by running the reference implementation's exact
// scan-and-track-limit algorithm standalone: on a tie, the higher (later
// in ascending order) layer wins.
func TestMultilayerNearest(t *testing.T) {
	cases := []struct {
		heights []int16
		worldZ  int32
		want    int // index into heights
	}{
		{[]int16{10, 20, 30}, 0, 0},
		{[]int16{10, 20, 30}, 10, 0},
		{[]int16{10, 20, 30}, 15, 1},
		{[]int16{10, 20, 30}, 20, 1},
		{[]int16{10, 20, 30}, 25, 2},
		{[]int16{10, 20, 30}, 100, 2},
		{[]int16{10, 20, 30}, -100, 0},
		{[]int16{10, 20, 30}, -15, 0},
		{[]int16{10, 20}, 0, 0},
		{[]int16{10, 20}, 10, 0},
		{[]int16{10, 20}, 15, 1},
		{[]int16{10, 20}, 20, 1},
		{[]int16{10, 20}, 25, 1},
		{[]int16{10, 20}, 100, 1},
		{[]int16{10, 20}, -100, 0},
		{[]int16{10, 20}, -15, 0},
		{[]int16{5, 15, 25}, 0, 0},
		{[]int16{5, 15, 25}, 10, 1},
		{[]int16{5, 15, 25}, 15, 1},
		{[]int16{5, 15, 25}, 20, 2},
		{[]int16{5, 15, 25}, 25, 2},
		{[]int16{5, 15, 25}, 100, 2},
		{[]int16{5, 15, 25}, -100, 0},
		{[]int16{5, 15, 25}, -15, 0},
		{[]int16{-20, -10, 0, 10, 20}, 0, 2},
		{[]int16{-20, -10, 0, 10, 20}, 10, 3},
		{[]int16{-20, -10, 0, 10, 20}, 15, 4},
		{[]int16{-20, -10, 0, 10, 20}, 20, 4},
		{[]int16{-20, -10, 0, 10, 20}, 25, 4},
		{[]int16{-20, -10, 0, 10, 20}, 100, 4},
		{[]int16{-20, -10, 0, 10, 20}, -100, 0},
		{[]int16{-20, -10, 0, 10, 20}, -15, 1},
	}

	for _, c := range cases {
		b := newTestMultilayer(t, c.heights)
		handle := b.Nearest(0, 0, c.worldZ)
		if got := b.Height(handle); got != c.heights[c.want] {
			t.Errorf("heights=%v worldZ=%d: Height(Nearest) = %d, want %d (index %d)",
				c.heights, c.worldZ, got, c.heights[c.want], c.want)
		}
		wantNSWE := NSWE((c.want % 15) + 1)
		if got := b.NSWE(handle); got != wantNSWE {
			t.Errorf("heights=%v worldZ=%d: NSWE(Nearest) = %v, want %v", c.heights, c.worldZ, got, wantNSWE)
		}
		if got := b.HeightNearest(0, 0, c.worldZ); got != c.heights[c.want] {
			t.Errorf("heights=%v worldZ=%d: HeightNearest = %d, want %d", c.heights, c.worldZ, got, c.heights[c.want])
		}
	}
}

// TestMultilayerAboveBelow checks Above/Below against vectors generated
// by running the reference implementation's exact top-down/bottom-up
// scan standalone.
func TestMultilayerAboveBelow(t *testing.T) {
	const noLayer = -1
	cases := []struct {
		heights              []int16
		worldZ               int32
		wantAbove, wantBelow int // index into heights, or noLayer
	}{
		{[]int16{10, 20, 30}, 0, 2, noLayer},
		{[]int16{10, 20, 30}, 10, 2, noLayer},
		{[]int16{10, 20, 30}, 15, 2, 0},
		{[]int16{10, 20, 30}, 20, 2, 0},
		{[]int16{10, 20, 30}, 25, 2, 1},
		{[]int16{10, 20, 30}, 100, noLayer, 2},
		{[]int16{10, 20, 30}, -100, 2, noLayer},
		{[]int16{10, 20, 30}, -15, 2, noLayer},
		{[]int16{10, 20}, 0, 1, noLayer},
		{[]int16{10, 20}, 10, 1, noLayer},
		{[]int16{10, 20}, 15, 1, 0},
		{[]int16{10, 20}, 20, noLayer, 0},
		{[]int16{10, 20}, 25, noLayer, 1},
		{[]int16{10, 20}, 100, noLayer, 1},
		{[]int16{10, 20}, -100, 1, noLayer},
		{[]int16{10, 20}, -15, 1, noLayer},
		{[]int16{5, 15, 25}, 0, 2, noLayer},
		{[]int16{5, 15, 25}, 10, 2, 0},
		{[]int16{5, 15, 25}, 15, 2, 0},
		{[]int16{5, 15, 25}, 20, 2, 1},
		{[]int16{5, 15, 25}, 25, noLayer, 1},
		{[]int16{5, 15, 25}, 100, noLayer, 2},
		{[]int16{5, 15, 25}, -100, 2, noLayer},
		{[]int16{5, 15, 25}, -15, 2, noLayer},
		{[]int16{-20, -10, 0, 10, 20}, 0, 4, 1},
		{[]int16{-20, -10, 0, 10, 20}, 10, 4, 2},
		{[]int16{-20, -10, 0, 10, 20}, 15, 4, 3},
		{[]int16{-20, -10, 0, 10, 20}, 20, noLayer, 3},
		{[]int16{-20, -10, 0, 10, 20}, 25, noLayer, 4},
		{[]int16{-20, -10, 0, 10, 20}, 100, noLayer, 4},
		{[]int16{-20, -10, 0, 10, 20}, -100, 4, noLayer},
		{[]int16{-20, -10, 0, 10, 20}, -15, 4, 0},
	}

	for _, c := range cases {
		b := newTestMultilayer(t, c.heights)

		above := b.Above(0, 0, c.worldZ)
		if c.wantAbove == noLayer {
			if above != -1 {
				t.Errorf("heights=%v worldZ=%d: Above = %d, want -1", c.heights, c.worldZ, above)
			}
		} else if got := b.Height(above); got != c.heights[c.wantAbove] {
			t.Errorf("heights=%v worldZ=%d: Height(Above) = %d, want %d", c.heights, c.worldZ, got, c.heights[c.wantAbove])
		}

		below := b.Below(0, 0, c.worldZ)
		if c.wantBelow == noLayer {
			if below != -1 {
				t.Errorf("heights=%v worldZ=%d: Below = %d, want -1", c.heights, c.worldZ, below)
			}
		} else if got := b.Height(below); got != c.heights[c.wantBelow] {
			t.Errorf("heights=%v worldZ=%d: Height(Below) = %d, want %d", c.heights, c.worldZ, got, c.heights[c.wantBelow])
		}
	}
}

func TestMultilayerBasics(t *testing.T) {
	b := newTestMultilayer(t, []int16{10, 20, 30})

	if got := b.Kind(); got != KindMultilayer {
		t.Errorf("Kind() = %v, want %v", got, KindMultilayer)
	}
	if !b.HasGeodata() {
		t.Errorf("HasGeodata() = false, want true")
	}
	if got := b.Layers(0, 0); got != 3 {
		t.Errorf("Layers(0,0) = %d, want 3", got)
	}
	if got := b.Layers(1, 1); got != 1 {
		t.Errorf("Layers(1,1) = %d, want 1", got)
	}

	// A different cell must resolve independently of cell (0,0)'s layers.
	if got := b.HeightNearest(1, 1, 999); got != 0 {
		t.Errorf("HeightNearest(1,1,999) = %d, want 0", got)
	}
}

func TestNewMultilayerRejectsInvalidLayerCounts(t *testing.T) {
	var zeroLayers [CellCount][]Cell
	for i := range zeroLayers {
		zeroLayers[i] = []Cell{{Height: 0, NSWE: AllDirections}}
	}
	zeroLayers[0] = nil // 0 layers: invalid.
	if _, err := NewMultilayer(zeroLayers); err == nil {
		t.Error("NewMultilayer with a 0-layer cell: got nil error, want error")
	}

	var tooMany [CellCount][]Cell
	for i := range tooMany {
		tooMany[i] = []Cell{{Height: 0, NSWE: AllDirections}}
	}
	over := make([]Cell, MaxLayers+1)
	tooMany[0] = over
	if _, err := NewMultilayer(tooMany); err == nil {
		t.Error("NewMultilayer with a cell over MaxLayers: got nil error, want error")
	}
}

// ---- from nswe_test.go ----
func TestNSWEString(t *testing.T) {
	cases := []struct {
		mask NSWE
		want string
	}{
		{NoDirections, "none"},
		{AllDirections, "all"},
		{North, "N"},
		{South, "S"},
		{West, "W"},
		{East, "E"},
		{North | South, "NS"},
		{West | East, "WE"},
		{North | West | East, "NWE"},
	}
	for _, c := range cases {
		if got := c.mask.String(); got != c.want {
			t.Errorf("NSWE(%#x).String() = %q, want %q", uint8(c.mask), got, c.want)
		}
	}
}

func TestNSWEAllows(t *testing.T) {
	cases := []struct {
		mask, dirs NSWE
		want       bool
	}{
		{AllDirections, North, true},
		{AllDirections, North | South, true},
		{North, South, false},
		{North | South, North | South, true},
		{North | South, North | East, false},
		{NoDirections, NoDirections, true},
	}
	for _, c := range cases {
		if got := c.mask.Allows(c.dirs); got != c.want {
			t.Errorf("NSWE(%#x).Allows(%#x) = %v, want %v", uint8(c.mask), uint8(c.dirs), got, c.want)
		}
	}
}

// Direction bit values are an exact wire contract (the on-disk NSWE
// nibble): East=0x01, West=0x02, South=0x04, North=0x08, verified against
// GeoStructure.java's CELL_FLAG_* constants.
func TestNSWEBitValues(t *testing.T) {
	cases := []struct {
		name string
		mask NSWE
		want uint8
	}{
		{"East", East, 0x01},
		{"West", West, 0x02},
		{"South", South, 0x04},
		{"North", North, 0x08},
		{"AllDirections", AllDirections, 0x0F},
		{"NoDirections", NoDirections, 0x00},
	}
	for _, c := range cases {
		if got := uint8(c.mask); got != c.want {
			t.Errorf("%s = %#x, want %#x", c.name, got, c.want)
		}
	}
}

// TestDecodeCell checks the per-cell code decode formula against vectors
// generated by running the reference implementation's exact arithmetic
// (data & 0x000F for NSWE; (short)((short)(data & 0xFFF0) >> 1) for
// height) standalone, including sign-extension edge cases.
func TestDecodeCell(t *testing.T) {
	cases := []struct {
		code       uint16
		wantNSWE   NSWE
		wantHeight int16
	}{
		{0x000F, 0xF, 0},
		{0x0001, 0x1, 0},
		{0x00D5, 0x5, 104},
		{0xFFF0, 0x0, -8},
		{0x0000, 0x0, 0},
		{0x8000, 0x0, -16384},
		{0x7FF0, 0x0, 16376},
		{0xFFFF, 0xF, -8},
		{0x000A, 0xA, 0},
		{0x0FF8, 0x8, 2040},
	}
	for _, c := range cases {
		got := DecodeCell(c.code)
		if got.NSWE != c.wantNSWE || got.Height != c.wantHeight {
			t.Errorf("DecodeCell(%#04x) = {NSWE:%#x Height:%d}, want {NSWE:%#x Height:%d}",
				c.code, uint8(got.NSWE), got.Height, uint8(c.wantNSWE), c.wantHeight)
		}
	}
}

// ---- from null_test.go ----
func TestNull(t *testing.T) {
	b := &Null{}

	if got := b.Kind(); got != KindNull {
		t.Errorf("Kind() = %v, want %v", got, KindNull)
	}
	if b.HasGeodata() {
		t.Errorf("HasGeodata() = true, want false")
	}
	if got := b.Layers(3, 4); got != 1 {
		t.Errorf("Layers(3,4) = %d, want 1", got)
	}

	// HeightNearest passes worldZ through unchanged, unlike every other
	// block kind, since there is no stored geodata to consult.
	for _, z := range []int32{0, 12345, -500} {
		if got := b.HeightNearest(0, 0, z); got != int16(z) {
			t.Errorf("HeightNearest(0,0,%d) = %d, want %d", z, got, int16(z))
		}
	}

	// Java's `(short) worldZ` conversion narrows out-of-range queries.
	if got := b.HeightNearest(0, 0, math.MaxInt32); got != -1 {
		t.Errorf("HeightNearest(0,0,MaxInt32) = %d, want -1", got)
	}
	if got := b.HeightNearest(0, 0, math.MinInt32); got != 0 {
		t.Errorf("HeightNearest(0,0,MinInt32) = %d, want 0", got)
	}

	if got := b.NSWENearest(0, 0, 0); got != AllDirections {
		t.Errorf("NSWENearest = %v, want all", got)
	}
	if got := b.Nearest(0, 0, 0); got != 0 {
		t.Errorf("Nearest = %d, want 0", got)
	}
	if got := b.Above(0, 0, 0); got != 0 {
		t.Errorf("Above = %d, want 0", got)
	}
	if got := b.Below(0, 0, 0); got != 0 {
		t.Errorf("Below = %d, want 0", got)
	}
	if got := b.Height(0); got != 0 {
		t.Errorf("Height(0) = %d, want 0", got)
	}
	if got := b.NSWE(0); got != AllDirections {
		t.Errorf("NSWE(0) = %v, want all", got)
	}
}

// ---- from region_test.go ----
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
	if got := r.Height(0, 2, r.Below(0, 2, 4, 5, 100)); got != 48 {
		t.Fatalf("multilayer below height = %d, want 48", got)
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
