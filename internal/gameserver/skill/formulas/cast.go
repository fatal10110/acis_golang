package formulas

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

// CastBreakRate returns the percent chance that incoming damage interrupts
// a magic cast before the final clamp and random roll. men must be a valid
// statbonus table index. attackCancel applies any already-resolved
// ATTACK_CANCEL stat modifiers; nil means no modifier.
func CastBreakRate(damage float64, men int, attackCancel func(float64) float64) float64 {
	base := 15 + math.Sqrt(13*damage) - (statbonus.MENBonus[men]*100 - 100)
	if attackCancel != nil {
		return attackCancel(base)
	}
	return base
}

// CastBreaks reports whether rate interrupts a cast for roll, a uniform
// random draw in [0, 100). Rates are clamped to the inclusive [1, 99]
// percent range before the comparison.
func CastBreaks(rate float64, roll int) bool {
	if rate < 1 {
		rate = 1
	} else if rate > 99 {
		rate = 99
	}
	return rate > float64(roll)
}
