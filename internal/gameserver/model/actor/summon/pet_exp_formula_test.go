package summon

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/vitals"
)

func TestPetDeathPenaltyMatchesGolden(t *testing.T) {
	tests := []struct {
		name            string
		level           int
		prev, cur, next int64
		exp             int64
	}{
		{"level 44", 44, 700000, 800000, 1000000, 900000},
		{"level 10", 10, 900, 1000, 2500, 1500},
		{"level 1", 1, 0, 0, 136, 100},
		// Past level ~93 the formula goes negative: a death at this level
		// grants exp instead of costing it, and negative halves round up.
		{"level 99 goes negative", 99, 4900000, 5000000, 5200000, 5100000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := vitals.PetDeath(t, tt.level, tt.prev, tt.cur, tt.next, tt.exp).Lost
			if got := petDeathPenalty(tt.level, tt.cur, tt.next); got != want {
				t.Errorf("petDeathPenalty(level=%d) = %d, want %d", tt.level, got, want)
			}
		})
	}
}

func TestPetRestoredExpMatchesGolden(t *testing.T) {
	tests := []struct {
		name           string
		expBeforeDeath int64
		currentExp     int64
		restorePercent float64
	}{
		{"half restore", 100000, 93160, 50},
		{"33 percent restore", 50000, 40000, 33},
		{"half point rounds up", 510, 481, 50},
		{"nothing lost", 100000, 100000, 100},
		{"no power", 510, 481, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := vitals.PetExpRestored(t, tt.expBeforeDeath, tt.currentExp, tt.restorePercent)
			if got := petRestoredExp(tt.expBeforeDeath, tt.currentExp, tt.restorePercent); got != want {
				t.Errorf("petRestoredExp() = %d, want %d", got, want)
			}
		})
	}
}
