package location

import "testing"

// A whole-number hypotenuse must sit exactly on the strict boundary: inside
// one unit further out, outside at the radius itself. The triples come from
// Euclid's formula, so every one has an integer ground distance.
func TestIn2DRadiusPythagoreanBoundary(t *testing.T) {
	for m := 2; m <= 140; m++ {
		for n := 1; n < m; n++ {
			if (m-n)%2 == 0 || gcd(m, n) != 1 {
				continue
			}
			a, b, c := m*m-n*n, 2*m*n, m*m+n*n
			for _, legs := range [][2]int{{a, b}, {b, a}, {-a, b}, {a, -b}} {
				origin := Location{X: 1000, Y: -2000, Z: 5}
				other := Location{X: origin.X + legs[0], Y: origin.Y + legs[1], Z: -900}
				if origin.In2DRadius(other, c) {
					t.Fatalf("In2DRadius(%v, %d) = true on the exact boundary", legs, c)
				}
				if !origin.In2DRadius(other, c+1) {
					t.Fatalf("In2DRadius(%v, %d) = false one unit inside", legs, c+1)
				}
				if origin.In2DRadius(other, c-1) {
					t.Fatalf("In2DRadius(%v, %d) = true one unit outside", legs, c-1)
				}
				if In2DRadius(origin.X, origin.Y, other.X, other.Y, c) {
					t.Fatalf("package In2DRadius(%v, %d) = true on the exact boundary", legs, c)
				}
			}
		}
	}
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
