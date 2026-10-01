package player

import "testing"

// TestCheckRecommendRefusalOrder pins the refusal order of a
// recommendation (RequestEvaluate.runImpl): self, level under 10, nothing
// left, target at 255, already recommended.
func TestCheckRecommendRefusalOrder(t *testing.T) {
	newChar := func(id int32, level, have, left int) *Character {
		c := &Character{ID: id, CharLevel: level}
		c.SetRecommendationCounts(have, left)
		return c
	}
	giver := newChar(1, 10, 0, 1)
	target := newChar(2, 1, 254, 0)

	cases := []struct {
		name   string
		giver  *Character
		target *Character
		want   RecommendRefusal
	}{
		{"self before level", newChar(3, 1, 0, 0), nil, RecommendSelf},
		{"level 9", newChar(4, 9, 0, 0), target, RecommendLevelTooLow},
		{"none left before a full target", newChar(5, 10, 0, 0), newChar(6, 1, 255, 0), RecommendNoneLeft},
		{"full target", giver, newChar(7, 1, 255, 0), RecommendTargetFull},
		{"accepted", giver, target, RecommendAccepted},
	}
	for _, tc := range cases {
		tgt := tc.target
		if tgt == nil {
			tgt = tc.giver
		}
		if got := tc.giver.CheckRecommend(tgt); got != tc.want {
			t.Errorf("%s: CheckRecommend = %d, want %d", tc.name, got, tc.want)
		}
	}

	if have, left := giver.Recommend(target); have != 255 || left != 0 {
		t.Fatalf("Recommend = have %d left %d, want 255 and 0", have, left)
	}
	giver.SetRecommendationCounts(0, 9)
	if got := giver.CheckRecommend(newChar(2, 1, 0, 0)); got != RecommendAlreadyGiven {
		t.Fatalf("CheckRecommend of a recommended id = %d, want RecommendAlreadyGiven", got)
	}
}

// TestRecommendationCountersClamp pins the counters' ranges: held 0..255,
// left 0..9.
func TestRecommendationCountersClamp(t *testing.T) {
	c := &Character{}
	c.SetRecommendationCounts(300, 12)
	if c.RecommendationsHave() != 255 || c.RecommendationsLeft() != 9 {
		t.Fatalf("clamped counters have %d left %d, want 255 and 9", c.RecommendationsHave(), c.RecommendationsLeft())
	}
	c.SetRecommendationCounts(-1, -1)
	if c.RecommendationsHave() != 0 || c.RecommendationsLeft() != 0 {
		t.Fatalf("clamped counters have %d left %d, want 0 and 0", c.RecommendationsHave(), c.RecommendationsLeft())
	}
}

func TestDailyRecommendations(t *testing.T) {
	for _, tc := range []struct{ level, left, loss int }{
		{1, 3, 1}, {19, 3, 1}, {20, 6, 2}, {39, 6, 2}, {40, 9, 3}, {80, 9, 3},
	} {
		if left, loss := DailyRecommendations(tc.level); left != tc.left || loss != tc.loss {
			t.Errorf("level %d: left %d loss %d, want %d and %d", tc.level, left, loss, tc.left, tc.loss)
		}
	}
	c := &Character{CharLevel: 40}
	c.SetRecommendationCounts(2, 0)
	c.RestoreRecommended([]int32{5})
	c.RefreshDailyRecommendations()
	if c.RecommendationsHave() != 0 || c.RecommendationsLeft() != 9 || c.hasRecommended(5) {
		t.Fatalf("after refresh have %d left %d recommended(5) %v, want 0, 9, false", c.RecommendationsHave(), c.RecommendationsLeft(), c.hasRecommended(5))
	}
}
