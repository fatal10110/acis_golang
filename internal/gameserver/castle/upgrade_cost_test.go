package castle

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// The gate and trap upgrade price tables, tripled under a dusk Seal of
// Strife and cut by a fifth, truncated, under a dawn one.
func TestUpgradeCosts(t *testing.T) {
	doors := []struct{ gateType, ratio, none, dusk, dawn int }{
		{1, 2, 300_000, 900_000, 240_000},
		{1, 3, 400_000, 1_200_000, 320_000},
		{1, 5, 500_000, 1_500_000, 400_000},
		{2, 2, 750_000, 2_250_000, 600_000},
		{2, 3, 900_000, 2_700_000, 720_000},
		{2, 5, 1_000_000, 3_000_000, 800_000},
		{3, 2, 1_600_000, 4_800_000, 1_280_000},
		{3, 3, 1_800_000, 5_400_000, 1_440_000},
		{3, 5, 2_000_000, 6_000_000, 1_600_000},
		{1, 4, 0, 0, 0},
		{4, 2, 0, 0, 0},
	}
	for _, d := range doors {
		for owner, want := range map[sevensigns.Cabal]int{sevensigns.NoCabal: d.none, sevensigns.Dusk: d.dusk, sevensigns.Dawn: d.dawn} {
			if got := DoorUpgradeCost(d.gateType, d.ratio, owner); got != want {
				t.Errorf("DoorUpgradeCost(%d, %d, %v) = %d, want %d", d.gateType, d.ratio, owner, got, want)
			}
		}
	}
	traps := []struct{ level, none, dusk, dawn int }{
		{1, 3_000_000, 9_000_000, 2_400_000},
		{2, 4_000_000, 12_000_000, 3_200_000},
		{3, 5_000_000, 15_000_000, 4_000_000},
		{4, 6_000_000, 18_000_000, 4_800_000},
		{0, 0, 0, 0},
		{5, 0, 0, 0},
	}
	for _, tr := range traps {
		for owner, want := range map[sevensigns.Cabal]int{sevensigns.NoCabal: tr.none, sevensigns.Dusk: tr.dusk, sevensigns.Dawn: tr.dawn} {
			if got := TrapUpgradeCost(tr.level, owner); got != want {
				t.Errorf("TrapUpgradeCost(%d, %v) = %d, want %d", tr.level, owner, got, want)
			}
		}
	}
}
