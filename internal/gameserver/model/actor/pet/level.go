package pet

import "math"

// roundInt64 rounds v to the nearest integer with halves toward positive
// infinity, matching the pet experience formulas.
func roundInt64(v float64) int64 {
	return int64(math.Floor(v + 0.5))
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
