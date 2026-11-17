package location

import "testing"

func TestIsFacingHalvesTheAngleInIntegers(t *testing.T) {
	facingEast := OrientedLocation{Heading: 0}
	ahead := Location{X: 100}
	slightlyOff := Location{X: 1000, Y: 21} // about 1.2 degrees off the heading
	behind := Location{X: -100}

	for _, tc := range []struct {
		name    string
		other   Location
		degrees int
		want    bool
	}{
		{"zero angle, exactly ahead", ahead, 0, true},
		{"zero angle, off the heading", slightlyOff, 0, false},
		{"angle 1 halves to 0, exactly ahead", ahead, 1, true},
		{"odd angle 3 halves to 1", slightlyOff, 3, false},
		{"even angle 4 halves to 2", slightlyOff, 4, true},
		{"negative angle", ahead, -2, false},
		{"full circle", behind, 360, true},
		{"over a full circle", behind, 720, true},
		{"half circle, behind", behind, 180, false},
	} {
		if got := facingEast.IsFacing(tc.other, tc.degrees); got != tc.want {
			t.Errorf("%s: IsFacing(%v, %d) = %v, want %v", tc.name, tc.other, tc.degrees, got, tc.want)
		}
	}
}
