package party

import "testing"

type body struct {
	x, y, z int
	radius  float64
}

func (b body) Position() (int, int, int) { return b.x, b.y, b.z }
func (b body) CollisionRadius() float64  { return b.radius }

// TestInRange pins the party range test: the range plus both collision
// radii, in 3D, inclusive; a range of 0 leaves only the radii, and -1 is
// unlimited.
func TestInRange(t *testing.T) {
	origin := body{radius: 8}
	for _, tc := range []struct {
		what  string
		rng   int
		other body
		want  bool
	}{
		{"edge of range plus radii", 1500, body{x: 1516, radius: 8}, true},
		{"just past it", 1500, body{x: 1517, radius: 8}, false},
		{"in 3D", 1500, body{x: 1000, z: 1200, radius: 8}, false},
		{"range 0, touching", 0, body{x: 16, radius: 8}, true},
		{"range 0, apart", 0, body{x: 17, radius: 8}, false},
		{"unlimited", -1, body{x: 1 << 20, radius: 8}, true},
	} {
		if got := InRange(tc.rng, origin, tc.other); got != tc.want {
			t.Errorf("%s: InRange = %v, want %v", tc.what, got, tc.want)
		}
	}
}
