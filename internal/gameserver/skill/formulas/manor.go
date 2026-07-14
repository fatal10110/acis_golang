package formulas

// HarvestSuccessRate returns the percent chance, floored at 1, that a
// harvest attempt succeeds: levelDiff is the absolute value of the
// harvester's level minus the seeded target's level. Every 1 level beyond
// a 5-level gap costs 5 percentage points.
func HarvestSuccessRate(levelDiff int) int {
	if levelDiff < 0 {
		levelDiff = -levelDiff
	}
	rate := 100
	if levelDiff > 5 {
		rate -= (levelDiff - 5) * 5
	}
	if rate < 1 {
		return 1
	}
	return rate
}

// SowSuccessRate returns the percent chance, floored at 1, that sowing a
// seed on a target succeeds: seedLevel/targetLevel/playerLevel are the
// seed's, target's and sower's levels; alternative reports whether the
// seed is an alternative-crop seed (lower base rate: 20 vs 90).
func SowSuccessRate(seedLevel, targetLevel, playerLevel int, alternative bool) int {
	minLevel := seedLevel - 5
	maxLevel := seedLevel + 5

	rate := 90
	if alternative {
		rate = 20
	}

	if targetLevel < minLevel {
		rate -= 5 * (minLevel - targetLevel)
	} else if targetLevel > maxLevel {
		rate -= 5 * (targetLevel - maxLevel)
	}

	diff := playerLevel - targetLevel
	if diff < 0 {
		diff = -diff
	}
	if diff > 5 {
		rate -= (diff - 5) * 5
	}

	if rate < 1 {
		return 1
	}
	return rate
}
