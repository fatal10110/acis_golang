package clan

import "testing"

func TestGraduationPoints(t *testing.T) {
	for _, tc := range []struct{ joined, want int }{
		{1, 400},
		{16, 400},
		{17, 390},
		{20, 360},
		{38, 180},
		{39, 170},
		{40, 170},
	} {
		if got := graduationPoints(tc.joined); got != tc.want {
			t.Errorf("graduationPoints(%d) = %d, want %d", tc.joined, got, tc.want)
		}
	}
}
