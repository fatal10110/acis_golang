package pet

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

// roundInt64 rounds v to the nearest integer, half away from zero, matching
// the rounding this package's exp/penalty formulas are specified with.
func roundInt64(v float64) int64 {
	return int64(math.Round(v))
}

// ExpForLevel returns the total experience required to reach level
// according to data's growth table, or (0, false) if data defines no row
// for that level.
func ExpForLevel(data *npc.PetData, level int) (int64, bool) {
	if data == nil {
		return 0, false
	}
	row, ok := data.Levels[level]
	if !ok {
		return 0, false
	}
	return row.MaxExp, true
}

// DeathPenaltyExpLoss returns the experience a pet at level loses on death:
// a percentage, decreasing as level rises, of the exp span between level
// and level+1. It returns 0 if data has no row for level or level+1.
func DeathPenaltyExpLoss(data *npc.PetData, level int) int64 {
	cur, ok := ExpForLevel(data, level)
	if !ok {
		return 0
	}
	next, ok := ExpForLevel(data, level+1)
	if !ok {
		return 0
	}

	percentLost := -0.07*float64(level) + 6.5
	return roundInt64(float64(next-cur) * percentLost / 100)
}

// RestoreExp returns the experience to add back when a death penalty is
// partially undone: restorePercent (0-100) of the gap between
// expBeforeDeath and the pet's exp right now.
func RestoreExp(expBeforeDeath, currentExp int64, restorePercent float64) int64 {
	return roundInt64(float64(expBeforeDeath-currentExp) * restorePercent / 100)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// SkillLevel returns the effective level a pet's own skill is cast at,
// given the pet's level and that skill's own maximum level. A pet's skill
// level rises by 1 every 10 pet levels up to level 69, then by 1 every 5
// levels from level 70 onward, clamped to [1, maxSkillLevel].
func SkillLevel(petLevel, maxSkillLevel int) int {
	var lvl int
	if petLevel < 70 {
		lvl = 1 + petLevel/10
	} else {
		lvl = 8 + (petLevel-70)/5
	}
	return clampInt(lvl, 1, maxSkillLevel)
}

// BabyPetSkillLevel returns the effective level a baby pet's own healing
// skills are cast at, given the pet's level. It scales faster below level
// 70 and caps at 12 regardless of the skill's own max level.
func BabyPetSkillLevel(petLevel int) int {
	if petLevel < 70 {
		return max(1, petLevel/10)
	}
	return min(12, 7+(petLevel-70)/5)
}
