package formulas

import "github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"

// SkillMasteryRate returns the value a uniform [0, 100) draw must stay
// under for a skill-mastery proc: the mastery stat times the INT bonus for
// a mage class, the STR bonus otherwise.
func SkillMasteryRate(power float64, mage bool, intStat, str int) float64 {
	statMul := statbonus.STRBonus[statbonus.ClampIndex(str)]
	if mage {
		statMul = statbonus.INTBonus[statbonus.ClampIndex(intStat)]
	}
	return power * statMul
}
