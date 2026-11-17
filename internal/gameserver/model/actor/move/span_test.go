package move

import "testing"

// TestGroundSpanIsExactOnPythagoreanTriples pins a ground leg's length to the
// square root of the summed squares: (20, 99) is exactly 101 units long, which
// a correctly rounded hypot misses by an ulp.
func TestGroundSpanIsExactOnPythagoreanTriples(t *testing.T) {
	for _, leg := range []struct{ dx, dy, want float64 }{{20, 99, 101}, {45, 108, 117}, {27, 120, 123}, {52, 165, 173}} {
		if got := span(MoveGround, leg.dx, leg.dy, 999); got != leg.want {
			t.Fatalf("span(MoveGround, %v, %v) = %v, want exactly %v", leg.dx, leg.dy, got, leg.want)
		}
	}
}
