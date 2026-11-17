package location

import (
	"math"
	"testing"
)

// TestDistance2DIsExactOnPythagoreanTriples pins the ground distance to the
// square root of the summed squares: on every integer right triangle it is
// exactly the hypotenuse, so the strict radius check leaves that hypotenuse
// out. (20, 99, 101) is one a correctly rounded hypot misses by an ulp.
func TestDistance2DIsExactOnPythagoreanTriples(t *testing.T) {
	origin := Location{X: 7, Y: -3, Z: 50}
	triples := 0
	for c := 2; c <= 1000; c++ {
		for a := 1; a < c; a++ {
			b2 := c*c - a*a
			b := int(math.Sqrt(float64(b2)))
			if b*b != b2 {
				continue
			}
			triples++
			at := Location{X: origin.X + a, Y: origin.Y - b, Z: -400}
			if got := origin.Distance2D(at); got != float64(c) {
				t.Fatalf("Distance2D over (%d, %d) = %v, want exactly %d", a, b, got, c)
			}
			if origin.In2DRadius(at, c) || In2DRadius(origin.X, origin.Y, at.X, at.Y, c) {
				t.Fatalf("In2DRadius over (%d, %d) at radius %d = true, want the boundary outside", a, b, c)
			}
			if !origin.In2DRadius(at, c+1) {
				t.Fatalf("In2DRadius over (%d, %d) at radius %d = false", a, b, c+1)
			}
		}
	}
	if triples == 0 {
		t.Fatal("no Pythagorean triple swept")
	}
	if got := (Location{}).Distance2D(Location{X: 20, Y: 99}); got != 101 {
		t.Fatalf("Distance2D over (20, 99) = %v, want 101", got)
	}
}
