package pet

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

func TestFeedConsume(t *testing.T) {
	stats := npc.PetLevelStats{MealInBattle: 12, MealInNormal: 3}

	if got := FeedConsume(true, stats); got != 12 {
		t.Errorf("FeedConsume(inCombat=true) = %d, want 12", got)
	}
	if got := FeedConsume(false, stats); got != 3 {
		t.Errorf("FeedConsume(inCombat=false) = %d, want 3", got)
	}
}

func TestNextFed(t *testing.T) {
	tests := []struct{ current, consume, want int }{
		{100, 10, 90},
		{5, 10, 0},  // floors at zero, never negative
		{10, 10, 0}, // exact consumption also floors at zero
	}
	for _, tt := range tests {
		if got := NextFed(tt.current, tt.consume); got != tt.want {
			t.Errorf("NextFed(%d, %d) = %d, want %d", tt.current, tt.consume, got, tt.want)
		}
	}
}

func TestBelowShare(t *testing.T) {
	tests := []struct {
		fed, maxMeal int
		share        float64
		want         bool
	}{
		{50, 100, 0.55, true},
		{60, 100, 0.55, false},
		// 100*0.55 is not exactly 55 in float64 (it rounds up very
		// slightly), so 55 does land below it. That's the same IEEE-754
		// double arithmetic the specified formula is defined in terms of.
		{55, 100, 0.55, true},
	}
	for _, tt := range tests {
		if got := BelowShare(tt.fed, tt.maxMeal, tt.share); got != tt.want {
			t.Errorf("BelowShare(%d, %d, %v) = %v, want %v", tt.fed, tt.maxMeal, tt.share, got, tt.want)
		}
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		fed, maxMeal int
		want         StarvationTier
	}{
		{0, 1000, StarvationSevere},
		{50, 1000, StarvationMinor}, // < 10% of 1000
		{99, 1000, StarvationMinor}, // just under the 10% line
		{100, 1000, StarvationNone}, // exactly 10% is not below it
		{500, 1000, StarvationNone},
	}
	for _, tt := range tests {
		if got := Classify(tt.fed, tt.maxMeal); got != tt.want {
			t.Errorf("Classify(%d, %d) = %v, want %v", tt.fed, tt.maxMeal, got, tt.want)
		}
	}
}

func TestStarvationTierLeaveChancePercent(t *testing.T) {
	tests := []struct {
		tier StarvationTier
		want int
	}{
		{StarvationNone, 0},
		{StarvationMinor, 3},
		{StarvationSevere, 30},
	}
	for _, tt := range tests {
		if got := tt.tier.LeaveChancePercent(); got != tt.want {
			t.Errorf("%v.LeaveChancePercent() = %d, want %d", tt.tier, got, tt.want)
		}
	}
}
