package pet

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"

// FeedConsume returns how many meal points a single feeding tick consumes
// from a pet's gauge, given its growth-table row for the current level.
// Meals burn faster while the pet's owner is in combat.
func FeedConsume(inCombat bool, levelStats npc.PetLevelStats) int {
	if inCombat {
		return levelStats.MealInBattle
	}
	return levelStats.MealInNormal
}

// NextFed returns the meal gauge after a feeding tick consumes consume
// points from current, floored at zero.
func NextFed(current, consume int) int {
	if current > consume {
		return current - consume
	}
	return 0
}

// BelowShare reports whether fed has dropped under share of maxMeal, e.g.
// share=0.55 tests the auto-feed threshold and share=0.10 the hungry
// threshold.
func BelowShare(fed, maxMeal int, share float64) bool {
	return float64(fed) < float64(maxMeal)*share
}

// StarvationTier classifies how close to starving a pet's meal gauge is,
// which governs the chance it abandons its owner on a feeding tick.
type StarvationTier int

const (
	// StarvationNone means the pet is not at risk of leaving its owner.
	StarvationNone StarvationTier = iota
	// StarvationMinor means the meal gauge is under 10% of its max but not
	// yet empty.
	StarvationMinor
	// StarvationSevere means the meal gauge is empty.
	StarvationSevere
)

// Classify returns the starvation tier for a meal gauge of fed out of
// maxMeal.
func Classify(fed, maxMeal int) StarvationTier {
	if fed == 0 {
		return StarvationSevere
	}
	if BelowShare(fed, maxMeal, 0.10) {
		return StarvationMinor
	}
	return StarvationNone
}

// LeaveChancePercent returns the percent chance, out of 100, that a pet
// abandons its owner on a feeding tick at this starvation tier. The caller
// rolls against it with the project's shared RNG helper.
func (t StarvationTier) LeaveChancePercent() int {
	switch t {
	case StarvationSevere:
		return 30
	case StarvationMinor:
		return 3
	default:
		return 0
	}
}
