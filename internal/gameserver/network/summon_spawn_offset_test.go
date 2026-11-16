package network

import "testing"

// TestRandomCompassDirectionDrawsEachEvenly draws many spawn directions:
// every draw is one of the eight compass directions, never the owner's own
// spot, and each direction comes up about an
// eighth of the time.
func TestRandomCompassDirectionDrawsEachEvenly(t *testing.T) {
	t.Parallel()
	const draws = 80_000
	counts := map[[2]int]int{}
	for range draws {
		dx, dy := randomCompassDirection()
		on := func(d int) bool { return d >= -1 && d <= 1 }
		if !on(dx) || !on(dy) || (dx == 0 && dy == 0) {
			t.Fatalf("randomCompassDirection() = (%d,%d), want one of the 8 directions", dx, dy)
		}
		counts[[2]int{dx, dy}]++
	}
	if len(counts) != 8 {
		t.Fatalf("directions drawn = %v, want all 8", counts)
	}
	// An eighth of the draws is 10000, with a standard deviation near 94.
	for dir, n := range counts {
		if n < 9_400 || n > 10_600 {
			t.Fatalf("direction %v drawn %d times in %d, want about %d", dir, n, draws, draws/8)
		}
	}
}
