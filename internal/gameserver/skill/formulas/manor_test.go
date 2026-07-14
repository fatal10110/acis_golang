package formulas

import "testing"

func TestHarvestSuccessRate(t *testing.T) {
	tests := []struct {
		name      string
		levelDiff int
		want      int
	}{
		{"no gap", 0, 100},
		{"gap at threshold", 5, 100},
		{"one past threshold", 6, 95},
		{"floored at 1", 25, 1},
		{"negative diff treated as absolute", -6, 95},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HarvestSuccessRate(tt.levelDiff); got != tt.want {
				t.Errorf("HarvestSuccessRate(%d) = %d, want %d", tt.levelDiff, got, tt.want)
			}
		})
	}
}

func TestSowSuccessRate(t *testing.T) {
	tests := []struct {
		name                                string
		seedLevel, targetLevel, playerLevel int
		alternative                         bool
		want                                int
	}{
		{"in range, normal seed", 40, 40, 40, false, 90},
		{"in range, alternative seed", 40, 40, 40, true, 20},
		{"target above seed range", 40, 50, 50, false, 65},
		{"target below seed range", 40, 25, 25, false, 40},
		{"player-target level gap", 40, 40, 60, false, 15},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SowSuccessRate(tt.seedLevel, tt.targetLevel, tt.playerLevel, tt.alternative)
			if got != tt.want {
				t.Errorf("SowSuccessRate() = %d, want %d", got, tt.want)
			}
		})
	}
}
