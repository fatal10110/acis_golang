package block

import "testing"

// TestRegionNearestScansInFileOrder pins the packed multilayer copy of the
// nearest-layer scan to the one Multilayer.Nearest runs: top-down, stopping
// at the first layer farther than the last, so an exact midpoint resolves
// to the lower layer and an out-of-order cell stops at its first local
// minimum.
func TestRegionNearestScansInFileOrder(t *testing.T) {
	for _, tc := range []struct {
		heights []int16 // bottom-up storage order
		z       int32
		want    int // index into heights
	}{
		// Packed heights keep whole multiples of CellHeight.
		{[]int16{16, 32}, 24, 0},
		{[]int16{16, 32}, 25, 1},
		{[]int16{16, 32, 48}, 40, 1},
		{[]int16{-3664, -3416}, -3540, 0},
		{[]int16{96, 0, 48}, 56, 2},
		{[]int16{96, 0, 48}, -8, 1},
	} {
		r, err := NewRegionFromBlocks([]Block{newTestMultilayer(t, tc.heights)})
		if err != nil {
			t.Fatalf("NewRegionFromBlocks: %v", err)
		}
		want := tc.heights[tc.want]
		if got := r.HeightNearest(0, 0, 0, 0, tc.z); got != want {
			t.Errorf("heights=%v z=%d: HeightNearest = %d, want %d", tc.heights, tc.z, got, want)
		}
		if got := r.NSWENearest(0, 0, 0, 0, tc.z); got != NSWE(tc.want%15+1) {
			t.Errorf("heights=%v z=%d: NSWENearest = %v, want %v", tc.heights, tc.z, got, NSWE(tc.want%15+1))
		}
		if got := r.Height(0, 0, r.Nearest(0, 0, 0, 0, tc.z)); got != want {
			t.Errorf("heights=%v z=%d: Height(Nearest) = %d, want %d", tc.heights, tc.z, got, want)
		}
	}
}
