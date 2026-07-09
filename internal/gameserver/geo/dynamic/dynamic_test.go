package dynamic

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

type stubObject struct {
	x, y, z int
	height  int
	data    [][]block.NSWE
}

func (o stubObject) GeoX() int               { return o.x }
func (o stubObject) GeoY() int               { return o.y }
func (o stubObject) GeoZ() int               { return o.z }
func (o stubObject) Height() int             { return o.height }
func (o stubObject) GeoData() [][]block.NSWE { return o.data }

type stubSampler struct {
	heights map[[2]int]int16
}

func (s stubSampler) HeightNearest(gx, gy, worldZ int) int16 {
	if h, ok := s.heights[[2]int{gx, gy}]; ok {
		return h
	}
	return int16(worldZ)
}

func (s stubSampler) Above(gx, gy, worldZ int) (int16, bool) {
	h, ok := s.heights[[2]int{gx, gy}]
	if !ok || int(h) <= worldZ {
		return 0, false
	}
	return h, true
}

func TestNearestLayerIndexReportsNotFoundOnEmptySlice(t *testing.T) {
	if _, ok := nearestLayerIndex(nil, 0); ok {
		t.Error("nearestLayerIndex(nil) ok = true, want false")
	}
	if _, ok := nearestLayerIndex([]block.Cell{}, 0); ok {
		t.Error("nearestLayerIndex(empty) ok = true, want false")
	}

	layers := []block.Cell{{Height: 10}, {Height: 20}}
	i, ok := nearestLayerIndex(layers, 19)
	if !ok || i != 1 {
		t.Errorf("nearestLayerIndex(non-empty) = (%d, %v), want (1, true)", i, ok)
	}
}

func TestCalculateGeoObject(t *testing.T) {
	inside := [][]bool{
		{false, false, false},
		{false, true, false},
		{false, false, false},
	}

	got := CalculateGeoObject(inside)

	if got[1][1] != block.NoDirections {
		t.Fatalf("center cell = %v, want none", got[1][1])
	}
	if got[0][1] != block.AllDirections&^block.East {
		t.Fatalf("west neighbor = %v, want east blocked", got[0][1])
	}
	if got[2][1] != block.AllDirections&^block.West {
		t.Fatalf("east neighbor = %v, want west blocked", got[2][1])
	}
	if got[1][0] != block.AllDirections&^block.South {
		t.Fatalf("north neighbor = %v, want south blocked", got[1][0])
	}
	if got[1][2] != block.AllDirections&^block.North {
		t.Fatalf("south neighbor = %v, want north blocked", got[1][2])
	}
	if got[0][0] != block.AllDirections {
		t.Fatalf("corner cell = %v, want unchanged", got[0][0])
	}
}

func TestBlockAddRemoveComplex(t *testing.T) {
	base := block.NewFlat(0)
	b := NewBlock(1, 2, base)
	obj := &stubObject{
		x:      8,
		y:      16,
		z:      0,
		height: 32,
		data: [][]block.NSWE{
			{block.AllDirections &^ block.South, block.NoDirections},
		},
	}

	b.Add(obj)

	if got := b.HeightNearest(0, 1, 0); got != 32 {
		t.Fatalf("inside cell height = %d, want 32", got)
	}
	if got := b.NSWENearest(0, 1, 0); got != block.NoDirections {
		t.Fatalf("inside cell nswe = %v, want none", got)
	}
	if got := b.NSWENearest(0, 0, 0); got != block.AllDirections&^block.South {
		t.Fatalf("neighbor nswe = %v, want south blocked", got)
	}
	if got := b.HeightNearestIgnore(0, 1, 0, obj); got != 0 {
		t.Fatalf("ignored height = %d, want original 0", got)
	}
	if got := b.NSWENearestIgnore(0, 1, 0, obj); got != block.AllDirections {
		t.Fatalf("ignored nswe = %v, want original all", got)
	}

	b.Remove(obj)

	if got := b.HeightNearest(0, 1, 0); got != 0 {
		t.Fatalf("removed height = %d, want 0", got)
	}
	if got := b.NSWENearest(0, 1, 0); got != block.AllDirections {
		t.Fatalf("removed nswe = %v, want all", got)
	}
}

func TestBlockAddMultilayerClampsToAboveLayer(t *testing.T) {
	var cells [block.CellCount][]block.Cell
	for i := range cells {
		cells[i] = []block.Cell{{Height: 0, NSWE: block.AllDirections}}
	}
	cells[0] = []block.Cell{
		{Height: 0, NSWE: block.AllDirections},
		{Height: 80, NSWE: block.AllDirections},
	}
	base, err := block.NewMultilayer(cells)
	if err != nil {
		t.Fatalf("NewMultilayer: %v", err)
	}
	b := NewBlock(0, 0, base)
	obj := &stubObject{
		x:      0,
		y:      0,
		z:      0,
		height: 100,
		data:   [][]block.NSWE{{block.NoDirections}},
	}

	b.Add(obj)

	if got := b.HeightNearest(0, 0, 0); got != 32 {
		t.Fatalf("inside multilayer height = %d, want 32", got)
	}
	if got := b.NSWENearest(0, 0, 0); got != block.NoDirections {
		t.Fatalf("inside multilayer nswe = %v, want none", got)
	}
}

func TestBlockDelegatesToBaseWhenUntouched(t *testing.T) {
	b := NewBlock(0, 0, &block.Null{})

	if b.HasGeodata() {
		t.Fatalf("HasGeodata = true, want false (delegated from Null)")
	}
	if got := b.HeightNearest(3, 4, 500); got != 500 {
		t.Fatalf("untouched height = %d, want 500 (delegated to Null's worldZ passthrough)", got)
	}

	obj := &stubObject{
		x:      0,
		y:      0,
		z:      0,
		height: 32,
		data:   [][]block.NSWE{{block.NoDirections}},
	}
	b.Add(obj)

	if got := b.HeightNearest(0, 0, 0); got != 32 {
		t.Fatalf("touched height = %d, want 32", got)
	}

	b.Remove(obj)

	if got := b.HeightNearest(0, 0, 777); got != 777 {
		t.Fatalf("height after remove = %d, want 777 (delegation restored, not stuck at the placeholder baseline)", got)
	}
}

func TestBlockConcurrentAddRemoveAndReads(t *testing.T) {
	b := NewBlock(0, 0, block.NewFlat(0))
	obj := &stubObject{
		x:      0,
		y:      0,
		z:      0,
		height: 32,
		data:   [][]block.NSWE{{block.NoDirections}},
	}

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				b.HeightNearest(0, 0, 0)
				b.NSWENearest(0, 0, 0)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 200; j++ {
			b.Add(obj)
			b.Remove(obj)
		}
	}()
	wg.Wait()
}

func TestDoorObjectFromTemplate(t *testing.T) {
	tmpl, err := door.NewTemplate(func() *mapSet {
		set := newMapSet()
		set.Set("id", 1)
		set.Set("name", "door")
		set.Set("type", "DOOR")
		set.Set("level", 1)
		set.Set("x", 24)
		set.Set("y", 40)
		set.Set("z", 0)
		set.Set("coords", []location.Point{
			{X: 16, Y: 32},
			{X: 48, Y: 32},
			{X: 48, Y: 64},
			{X: 16, Y: 64},
		})
		set.Set("hp", 1)
		set.Set("pDef", 1)
		set.Set("mDef", 1)
		set.Set("height", 80)
		return set
	}().StatSet)
	if err != nil {
		t.Fatalf("NewTemplate: %v", err)
	}

	obj, err := NewDoorObject(tmpl, stubSampler{
		heights: map[[2]int]int16{
			{8194, 16387}: 96,
			{8193, 16386}: 64,
		},
	})
	if err != nil {
		t.Fatalf("NewDoorObject: %v", err)
	}

	if obj.GeoX() != 8192 || obj.GeoY() != 16385 {
		t.Fatalf("geo origin = (%d,%d), want (8192,16385)", obj.GeoX(), obj.GeoY())
	}
	if obj.GeoZ() != 64 {
		t.Fatalf("geo z = %d, want 64", obj.GeoZ())
	}
	if obj.Height() != 80 {
		t.Fatalf("height = %d, want 80", obj.Height())
	}
	if len(obj.GeoData()) != 5 || len(obj.GeoData()[0]) != 5 {
		t.Fatalf("geoData dims = %dx%d, want 5x5", len(obj.GeoData()), len(obj.GeoData()[0]))
	}
	if obj.GeoData()[2][1] != block.NoDirections {
		t.Fatalf("interior cell = %v, want none", obj.GeoData()[2][1])
	}
	if obj.GeoData()[2][0] != block.North|block.West|block.East {
		t.Fatalf("edge cell = %v, want NWE", obj.GeoData()[2][0])
	}
}
