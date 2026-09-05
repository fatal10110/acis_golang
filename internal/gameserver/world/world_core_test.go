package world

import (
	"fmt"
	"testing"
)

// ---- from grid_test.go ----
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

type regionTestObject struct {
	Presence
	id int32
}

func (o *regionTestObject) ObjectID() int32 { return o.id }

type regionTestPlayer struct {
	regionTestObject
}

func (p *regionTestPlayer) WorldPlayer() {}

func TestRegion_AddReplaceSameID(t *testing.T) {
	r := newRegion(0, 0)
	first := &regionTestObject{id: 7}
	second := &regionTestObject{id: 7}
	r.Add(first)
	r.Add(second)

	got := r.AppendObjects(nil)
	if len(got) != 1 {
		t.Fatalf("AppendObjects after same-id Add = %d objects, want 1", len(got))
	}
	if got[0] != second {
		t.Fatalf("same-id Add kept %p, want later object %p", got[0], second)
	}
}

func TestRegion_RemoveAndRemoveIfSame(t *testing.T) {
	r := newRegion(0, 0)
	a := &regionTestObject{id: 1}
	b := &regionTestObject{id: 2}
	c := &regionTestObject{id: 3}
	r.Add(a)
	r.Add(b)
	r.Add(c)

	r.Remove(2)
	if got := objectIDs(r.AppendObjects(nil)); !sameIDs(got, []int32{1, 3}) {
		t.Fatalf("after Remove(2) ids = %v, want [1 3]", got)
	}

	r.Remove(99)
	if got := objectIDs(r.AppendObjects(nil)); !sameIDs(got, []int32{1, 3}) {
		t.Fatalf("Remove missing id changed set to %v", got)
	}

	other := &regionTestObject{id: 1}
	if r.removeIfSame(1, other) {
		t.Fatal("removeIfSame dropped a different object sharing the id")
	}
	if !r.removeIfSame(1, a) {
		t.Fatal("removeIfSame did not drop the registered object")
	}
	if got := objectIDs(r.AppendObjects(nil)); !sameIDs(got, []int32{3}) {
		t.Fatalf("after removeIfSame ids = %v, want [3]", got)
	}
	if r.removeIfSame(3, a) {
		t.Fatal("removeIfSame succeeded for an object that is not registered")
	}
}

func TestRegion_appendObjectsExcept(t *testing.T) {
	r := newRegion(0, 0)
	r.Add(&regionTestObject{id: 1})
	r.Add(&regionTestObject{id: 2})
	r.Add(&regionTestObject{id: 3})

	got := objectIDs(r.appendObjectsExcept(nil, 2))
	if !sameIDs(got, []int32{1, 3}) {
		t.Fatalf("appendObjectsExcept(2) = %v, want [1 3]", got)
	}
	got = objectIDs(r.appendObjectsExcept(nil, 99))
	if !sameIDs(got, []int32{1, 2, 3}) {
		t.Fatalf("appendObjectsExcept missing id = %v, want [1 2 3]", got)
	}
}

func TestRegion_playersCountFollowsPlayerAddRemove(t *testing.T) {
	r := newRegion(0, 0)
	p := &regionTestPlayer{regionTestObject{id: 10}}
	npc := &regionTestObject{id: 11}
	r.Add(p)
	r.Add(npc)
	if n := r.playersCount.Load(); n != 1 {
		t.Fatalf("playersCount after Add player+npc = %d, want 1", n)
	}
	r.Remove(11)
	if n := r.playersCount.Load(); n != 1 {
		t.Fatalf("playersCount after Remove npc = %d, want 1", n)
	}
	if !r.removeIfSame(10, p) {
		t.Fatal("removeIfSame did not drop player")
	}
	if n := r.playersCount.Load(); n != 0 {
		t.Fatalf("playersCount after removeIfSame player = %d, want 0", n)
	}
}

func TestRegion_AppendObjectsConcurrentWithMutations(t *testing.T) {
	r := newRegion(0, 0)
	for i := int32(1); i <= 32; i++ {
		r.Add(&regionTestObject{id: i})
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		var buf []Tracked
		for i := 0; i < 1000; i++ {
			buf = r.AppendObjects(buf[:0])
			_ = r.appendObjectsExcept(buf[:0], 1)
		}
	}()
	for i := int32(1); i <= 32; i++ {
		r.Remove(i)
		r.Add(&regionTestObject{id: i})
	}
	<-done
}

func objectIDs(objs []Tracked) []int32 {
	ids := make([]int32, len(objs))
	for i, o := range objs {
		ids[i] = o.ObjectID()
	}
	return ids
}

func sameIDs(got, want []int32) bool {
	if len(got) != len(want) {
		return false
	}
	counts := make(map[int32]int, len(want))
	for _, id := range want {
		counts[id]++
	}
	for _, id := range got {
		counts[id]--
		if counts[id] < 0 {
			return false
		}
	}
	return true
}

func knownListFixture(tb testing.TB, n int) (*State, Tracked) {
	tb.Helper()
	s := New()
	observer := &regionTestObject{id: 1}
	s.Spawn(observer, 0, 0, 0, 0)
	for i := 0; i < n; i++ {
		o := &regionTestObject{id: int32(i + 2)}
		bucket := i % 9
		x := (bucket%3 - 1) * regionSize
		y := (bucket/3 - 1) * regionSize
		s.Spawn(o, x, y, 0, 0)
	}
	return s, observer
}

func BenchmarkAppendKnown(b *testing.B) {
	for _, n := range []int{50, 300, 1500} {
		b.Run(fmt.Sprintf("%d", n), func(b *testing.B) {
			s, observer := knownListFixture(b, n)
			var buf []Tracked
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				buf = s.AppendKnown(buf[:0], observer)
			}
		})
	}
}

func BenchmarkForEachKnownInRadius(b *testing.B) {
	for _, n := range []int{50, 300, 1500} {
		b.Run(fmt.Sprintf("%d", n), func(b *testing.B) {
			s, observer := knownListFixture(b, n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				s.ForEachKnownInRadius(observer, -1, func(Tracked) {})
			}
		})
	}
}
