package warrior

import "testing"

// TestFleeLineFollowsTheDrawBounds checks the NpcString a fleeing runner to
// its clan shouts at each edge of the draw bounds: 7 draws for each of
// 1000007 to 1000017, 2 for each of 1000018 to 1000026, and 1000027 from 95.
func TestFleeLineFollowsTheDrawBounds(t *testing.T) {
	for _, row := range []struct {
		draw int
		want int32
	}{
		{0, 1000007},
		{6, 1000007},
		{7, 1000008},
		{13, 1000008},
		{14, 1000009},
		{69, 1000016},
		{70, 1000017},
		{76, 1000017},
		{77, 1000018},
		{78, 1000018},
		{79, 1000019},
		{92, 1000025},
		{93, 1000026},
		{94, 1000026},
		{95, 1000027},
		{99, 1000027},
	} {
		if got := fleeLine(row.draw); got != row.want {
			t.Errorf("fleeLine(%d) = %d, want %d", row.draw, got, row.want)
		}
	}
}
