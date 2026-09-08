package world

import (
	"fmt"
	"testing"
)

// relocateBenchPlayer is a Player+Observer double for the region-crossing
// hot path. Discover/Forget are no-ops so the benchmark isolates relocate's
// own allocations.
type relocateBenchPlayer struct {
	Presence
	RelocateScratch
	id int32
}

func (p *relocateBenchPlayer) ObjectID() int32  { return p.id }
func (p *relocateBenchPlayer) Discover(Tracked) {}
func (p *relocateBenchPlayer) Forget(Tracked)   {}
func (p *relocateBenchPlayer) WorldPlayer()     {}

type relocateBenchObserver struct {
	Presence
	id int32
}

func (o *relocateBenchObserver) ObjectID() int32  { return o.id }
func (o *relocateBenchObserver) Discover(Tracked) {}
func (o *relocateBenchObserver) Forget(Tracked)   {}

func regionCenter(tileX, tileY int) (x, y int) {
	return MinX + tileX*regionSize + regionSize/2, MinY + tileY*regionSize + regionSize/2
}

// setupRelocateBench seeds n Observer-tagged neighbors into the three
// regions a player leaves and the three it enters when walking one region
// east from an interior tile, so every neighbor produces Discover/Forget
// work on the crossing.
func setupRelocateBench(n int) (s *State, p *relocateBenchPlayer, x0, y, x1 int) {
	s = New()
	const tileX, tileY = 10, 10
	x0, y = regionCenter(tileX, tileY)
	x1, _ = regionCenter(tileX+1, tileY)

	// Stationary players on the shared band keep leave and enter columns
	// active so setActive/notifyActivity do not run on the crossing.
	s.Spawn(&relocateBenchPlayer{id: int32(n + 10)}, x0, y, 0, 0)
	s.Spawn(&relocateBenchPlayer{id: int32(n + 11)}, x1, y, 0, 0)

	leave := [3][2]int{{tileX - 1, tileY - 1}, {tileX - 1, tileY}, {tileX - 1, tileY + 1}}
	enter := [3][2]int{{tileX + 2, tileY - 1}, {tileX + 2, tileY}, {tileX + 2, tileY + 1}}
	half := n / 2
	for i := 0; i < n; i++ {
		var tx, ty int
		if i < half {
			cell := leave[i%3]
			tx, ty = cell[0], cell[1]
		} else {
			cell := enter[i%3]
			tx, ty = cell[0], cell[1]
		}
		ox, oy := regionCenter(tx, ty)
		s.Spawn(&relocateBenchObserver{id: int32(i + 2)}, ox, oy, 0, 0)
	}

	p = &relocateBenchPlayer{id: 1}
	s.Spawn(p, x0, y, 0, 0)
	return s, p, x0, y, x1
}

func TestRelocatePlayerRegionCrossingAllocs(t *testing.T) {
	// p embeds RelocateScratch, so relocate reuses its notifications and
	// objects buffers across crossings instead of making fresh ones per
	// call. Bound is loose on purpose: it must catch a regression back to
	// grow-from-nil (11+ allocs at 300, 17+ at 1500) or back to a
	// per-crossing make (2 at 300/1500, see #2288) without depending on
	// exact inlining or GC timing. The residual 1 alloc/op is a baseline
	// relocate cost unrelated to the notifications/objects scratch —
	// present even at nearby=50 before this issue's change.
	for _, tt := range []struct {
		n   int
		max float64
	}{
		{300, 1},
		{1500, 1},
	} {
		t.Run(fmt.Sprintf("nearby=%d", tt.n), func(t *testing.T) {
			s, p, x0, y, x1 := setupRelocateBench(tt.n)
			i := 0
			var moveErr error
			got := testing.AllocsPerRun(100, func() {
				i++
				dst := x1
				if i%2 == 1 {
					dst = x0
				}
				moveErr = s.Move(p, dst, y, 0)
			})
			if moveErr != nil {
				t.Fatal(moveErr)
			}
			if got > tt.max {
				t.Fatalf("relocate allocations per player region crossing = %v, want <= %v (grow-from-nil was 11 at 300 / 17 at 1500)", got, tt.max)
			}
		})
	}
}

func BenchmarkRelocatePlayerRegionCrossing(b *testing.B) {
	for _, n := range []int{50, 300, 1500} {
		b.Run(fmt.Sprintf("nearby=%d", n), func(b *testing.B) {
			s, p, x0, y, x1 := setupRelocateBench(n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				dstX := x1
				if i%2 == 1 {
					dstX = x0
				}
				if err := s.Move(p, dstX, y, 0); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
