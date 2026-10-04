package fence

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

type counterIDs struct{ next int32 }

func (c *counterIDs) NextID() (int32, error) {
	c.next++
	return c.next, nil
}

// flatEngine is an engine whose first region is flat ground at height 0.
func flatEngine(t *testing.T) *engine.Engine {
	t.Helper()
	blocks := make([]block.Block, block.RegionBlockCount)
	flat := block.NewFlat(0)
	for i := range blocks {
		blocks[i] = flat
	}
	region, err := block.NewRegionFromBlocks(blocks)
	if err != nil {
		t.Fatalf("NewRegionFromBlocks: %v", err)
	}
	e := engine.New()
	if err := e.SetRegion(engine.TileXMin, engine.TileYMin, region); err != nil {
		t.Fatalf("SetRegion: %v", err)
	}
	return e
}

// TestSizeAndAlignment pins FenceManager.getFenceSize's 100-unit steps
// (under 199, 299, ... 1099) and the coordinate alignment
// x & 0xFFFFFFF0 + offset, which Java evaluates as x & (0xFFFFFFF0 +
// offset). The expected values were worked out with Java int arithmetic.
func TestSizeAndAlignment(t *testing.T) {
	for _, tc := range []struct {
		n     int
		want  size
		known bool
	}{
		{-100, size{8, 11}, true},
		{0, size{8, 11}, true},
		{198, size{8, 11}, true},
		{199, size{0, 18}, true},
		{300, size{0, 24}, true},
		{700, size{8, 49}, true},
		{998, size{8, 61}, true},
		{999, size{0, 68}, true},
		{1098, size{0, 68}, true},
		{1099, size{}, false},
	} {
		got, ok := sizeOf(tc.n)
		if got != tc.want || ok != tc.known {
			t.Errorf("sizeOf(%d) = %v %v, want %v %v", tc.n, got, ok, tc.want, tc.known)
		}
	}
	for _, tc := range []struct{ v, offset, want int }{
		{1005, 0, 992},
		{1005, 8, 1000},
		{-71433, 0, -71440},
		{-71433, 8, -71440},
		{-71441, 0, -71456},
		{-71441, 8, -71448},
	} {
		if got := align(tc.v, size{offset: tc.offset}); got != tc.want {
			t.Errorf("align(%d, offset %d) = %d, want %d", tc.v, tc.offset, got, tc.want)
		}
	}
}

// TestOutline pins the inner description FenceManager.addFence builds for
// an 11x11-cell fence: type 2 is a ring two cells thick, one cell in from
// the edge; any other type keeps only its four 2x2 corners.
func TestOutline(t *testing.T) {
	ring := []string{
		"...........",
		".#########.",
		".#########.",
		".##.....##.",
		".##.....##.",
		".##.....##.",
		".##.....##.",
		".##.....##.",
		".#########.",
		".#########.",
		"...........",
	}
	corners := []string{
		"...........",
		".##.....##.",
		".##.....##.",
		"...........",
		"...........",
		"...........",
		"...........",
		"...........",
		".##.....##.",
		".##.....##.",
		"...........",
	}
	for _, tc := range []struct {
		typ  int
		want []string
	}{{2, ring}, {1, corners}, {0, corners}} {
		inside := outline(tc.typ, 11, 11)
		for iy, row := range tc.want {
			var got strings.Builder
			for ix := range 11 {
				if inside[ix][iy] {
					got.WriteByte('#')
				} else {
					got.WriteByte('.')
				}
			}
			if got.String() != row {
				t.Errorf("type %d row %d = %s, want %s", tc.typ, iy, got.String(), row)
			}
		}
	}
}

// TestAddBlocksMovementUntilRemoved places a 100x300 two-layer fence of
// type 2 on flat ground: it and its layer join the world at the aligned
// position, its ring blocks movement out of and into it and the cells
// beside the ring lose only the edge facing it, and removing it restores
// the ground and takes both objects out of the world.
func TestAddBlocksMovementUntilRemoved(t *testing.T) {
	e := flatEngine(t)
	state := world.New()
	m := NewManager(e, state, &counterIDs{next: 100})

	x, y := engine.WorldXMin+2005, engine.WorldYMin+2003
	f, err := m.Add(x, y, 0, 2, 100, 300, 2)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	// 100 units align with offset 8, 300 with offset 0.
	if fx, fy, fz := f.Position(); fx != engine.WorldXMin+2000 || fy != engine.WorldYMin+2000 || fz != 0 {
		t.Fatalf("fence at (%d, %d, %d), want (%d, %d, 0)", fx, fy, fz, engine.WorldXMin+2000, engine.WorldYMin+2000)
	}
	if f.ObjectID() != 101 || len(f.layers) != 1 || f.layers[0].ObjectID() != 102 {
		t.Fatalf("fence id %d with %d layer(s), want 101 with layer 102", f.ObjectID(), len(f.layers))
	}
	for _, id := range []int32{101, 102} {
		if _, ok := state.Object(id); !ok {
			t.Fatalf("object %d not in the world", id)
		}
	}
	shape := f.shape
	if shape.GeoX() != 120 || shape.GeoY() != 113 || shape.GeoZ() != 0 || shape.Height() != 48 {
		t.Fatalf("shape at (%d, %d, %d) height %d, want (120, 113, 0) height 48", shape.GeoX(), shape.GeoY(), shape.GeoZ(), shape.Height())
	}

	// The fence spans cells x 120..130, y 113..136; its middle is open.
	center := [2]int{engine.WorldX(125), engine.WorldY(125)}
	south := [2]int{engine.WorldX(125), engine.WorldY(145)}
	west := [2]int{engine.WorldX(110), engine.WorldY(125)}
	if e.CanMove(center[0], center[1], 0, south[0], south[1], 0) {
		t.Fatal("moved out of the fence through its ring")
	}
	if e.CanMove(west[0], west[1], 0, center[0], center[1], 0) {
		t.Fatal("moved into the fence through its ring")
	}
	if !e.CanMove(west[0], west[1], 0, engine.WorldX(110), engine.WorldY(145), 0) {
		t.Fatal("blocked beside the fence")
	}
	// x 120 is the outer column: it may be walked along but not east into
	// the ring at x 121.
	if got := e.NSWENearest(120, 125, 0); got != block.AllDirections&^block.East {
		t.Fatalf("outer cell NSWE = %#x, want all but east", got)
	}
	if got := e.NSWENearest(125, 125, 0); got != block.AllDirections {
		t.Fatalf("middle cell NSWE = %#x, want all", got)
	}

	if !m.Remove(f) || m.Remove(f) {
		t.Fatal("Remove should succeed once")
	}
	if !e.CanMove(center[0], center[1], 0, south[0], south[1], 0) {
		t.Fatal("still blocked after the fence was removed")
	}
	for _, id := range []int32{101, 102} {
		if _, ok := state.Object(id); ok {
			t.Fatalf("object %d still in the world", id)
		}
	}
	if len(m.Fences()) != 0 {
		t.Fatalf("fences = %d after removal, want 0", len(m.Fences()))
	}
}

// TestAddRejects pins the fences FenceManager.addFence refuses before
// taking an id: ones reaching past the world edge (also through a width
// whose 32-bit sum wraps) and one longer than the size table.
func TestAddRejects(t *testing.T) {
	ids := &counterIDs{}
	m := NewManager(engine.New(), world.New(), ids)
	if _, err := m.Add(engine.WorldXMax-50, 0, 0, 2, 100, 100, 1); !errors.Is(err, errOutOfWorld) {
		t.Fatalf("past the world edge: err = %v, want errOutOfWorld", err)
	}
	// -100000 + -2147483600 wraps in 32 bits to 2147383696, past the
	// world's east edge: refused, not placed with a negative width.
	if _, err := m.Add(-100000, 0, 0, 2, -2147483600, 100, 1); !errors.Is(err, errOutOfWorld) {
		t.Fatalf("width wrapping past the world edge: err = %v, want errOutOfWorld", err)
	}
	if _, err := m.Add(0, 0, 0, 2, 1100, 100, 1); !errors.Is(err, errUnknownSize) {
		t.Fatalf("1100 wide: err = %v, want errUnknownSize", err)
	}
	if ids.next != 0 || len(m.Fences()) != 0 {
		t.Fatalf("rejected fences took %d id(s) and listed %d", ids.next, len(m.Fences()))
	}
}

// TestFencesInPlacementOrder pins the list order //listfence shows, and
// that a fence one layer high, or none, has no extra layers while three
// layers are the most.
func TestFencesInPlacementOrder(t *testing.T) {
	m := NewManager(engine.New(), world.New(), &counterIDs{})
	var placed []*Fence
	for _, height := range []int{1, 0, 3, 5} {
		f, err := m.Add(0, 0, 0, 1, 200, 200, height)
		if err != nil {
			t.Fatalf("Add height %d: %v", height, err)
		}
		placed = append(placed, f)
	}
	if !slices.Equal(m.Fences(), placed) {
		t.Fatal("Fences() not in placement order")
	}
	for i, want := range []int{0, 0, 2, 2} {
		if got := len(placed[i].layers); got != want {
			t.Errorf("fence %d layers = %d, want %d", i, got, want)
		}
	}
}
