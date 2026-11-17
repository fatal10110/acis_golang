package network

import (
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// TestDrownDamageIsMaxHPTimesBreathMultiplierTimesRate pins the per-second
// drowning damage with hand-computed amounts: the fighter rate is 0.001724,
// the mage rate 0.002698, scaled by the race's breath multiplier.
func TestDrownDamageIsMaxHPTimesBreathMultiplierTimesRate(t *testing.T) {
	for _, tc := range []struct {
		name  string
		maxHP float64
		race  player.Race
		mage  bool
		want  float64
	}{
		{"human fighter", 3000, player.RaceHuman, false, 5.172},
		{"human mage", 3000, player.RaceHuman, true, 8.094},
		{"elf fighter", 3000, player.RaceElf, false, 7.758},
		{"dark elf mage", 3000, player.RaceDarkElf, true, 12.141},
		{"orc fighter", 3000, player.RaceOrc, false, 4.6548},
		{"dwarf fighter", 3000, player.RaceDwarf, false, 4.1376},
		{"zero max HP", 0, player.RaceHuman, false, 0},
	} {
		if got := drownDamage(tc.maxHP, tc.race, tc.mage); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%s: drownDamage() = %v, want %v", tc.name, got, tc.want)
		}
	}
}
