package zaken

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// TestFartherDoesNotWrap: a ground distance whose square leaves 32 bits
// (an axis gap of 46,341 and over) is still far, up to the 65,536 gap
// whose 32-bit square is exactly 0 (#5131).
func TestFartherDoesNotWrap(t *testing.T) {
	b := script.Loc{X: 55000, Y: 218000, Z: -3216}
	for _, tc := range []struct {
		dx, dy int
		r      int32
		want   bool
	}{
		{dx: 250, r: 250, want: false},
		{dx: 251, r: 250, want: true},
		{dx: 150, dy: 200, r: 250, want: false},
		{dx: 1500, r: 1500, want: false},
		{dx: 46340, r: 250, want: true},
		{dx: 46341, r: 250, want: true},
		{dx: 46341, r: 1500, want: true},
		{dx: 65535, r: 250, want: true},
		{dx: 65536, r: 250, want: true},
		{dx: -65536, r: 1500, want: true},
		{dy: 65536, r: 1500, want: true},
		{dx: 46341, dy: 46341, r: 1500, want: true},
	} {
		a := script.Loc{X: b.X + tc.dx, Y: b.Y + tc.dy, Z: b.Z}
		if got := farther(a, b, tc.r); got != tc.want {
			t.Errorf("farther(gap %d,%d, r %d) = %v, want %v", tc.dx, tc.dy, tc.r, got, tc.want)
		}
	}
}
