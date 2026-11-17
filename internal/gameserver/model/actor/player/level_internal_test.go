package player

import "testing"

func TestLevelForExpNeverPassesTheSentinelLevel(t *testing.T) {
	table, err := NewLevelTable(map[int]Level{
		1: {RequiredExpToLevelUp: 0},
		2: {RequiredExpToLevelUp: 100},
		3: {RequiredExpToLevelUp: 300},
		4: {RequiredExpToLevelUp: 600},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		exp  int64
		want int
	}{
		{exp: 0, want: 1},
		{exp: 99, want: 1},
		{exp: 100, want: 2},
		{exp: 599, want: 3},
		{exp: 600, want: 4},
		{exp: 1 << 40, want: 4},
	} {
		if got := table.levelForExp(tc.exp); got != tc.want {
			t.Errorf("levelForExp(%d) = %d, want %d", tc.exp, got, tc.want)
		}
	}
}
