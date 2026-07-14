package formulas

// DoorUnlockSpecialSucceeds reports whether a skill that ignores a door's
// normal unlock levels opens it outright, given roll, a uniform random
// draw in [0, 100).
func DoorUnlockSpecialSucceeds(power float64, roll int) bool {
	return float64(roll) < power
}

// DoorUnlockRate returns the out-of-120 threshold a regular unlock skill's
// level must beat to open a door: 0 at level 0 (never opens), rising
// through levels 1-3, and maxed from level 4 up.
func DoorUnlockRate(level int) int {
	switch level {
	case 0:
		return 0
	case 1:
		return 30
	case 2:
		return 50
	case 3:
		return 75
	default:
		return 100
	}
}

// DoorUnlockSucceeds reports whether a regular unlock skill's roll opens a
// door, given roll, a uniform random draw in [0, 120).
func DoorUnlockSucceeds(level int, roll int) bool {
	return roll < DoorUnlockRate(level)
}

// ChestUnlockDeluxeKeyRate returns the percent chance a deluxe-key-branded
// unlock skill opens a chest: level is the chest's status level,
// skillLevel is the unlock skill's level, regularKey reports whether the
// cast skill is the base regular key (60% starting chance) rather than
// any other deluxe key (100% starting chance). Each level of mismatch
// between the chest and the key costs 40 percentage points.
func ChestUnlockDeluxeKeyRate(level, skillLevel int, regularKey bool) int {
	keyLevelNeeded := (level / 10) - skillLevel
	if keyLevelNeeded < 0 {
		keyLevelNeeded = -keyLevelNeeded
	}

	base := 100
	if regularKey {
		base = 60
	}
	return base - keyLevelNeeded*40
}

// ChestUnlockRate returns the percent chance a regular (non-deluxe-key)
// unlock skill opens a chest at the given status level and unlock skill
// level. definite is true when the outcome doesn't depend on any roll at
// all: the skill level is too low for the level bracket (guaranteed
// failure, succeeds false) or high enough to bypass the percent-chance
// cap (guaranteed success, succeeds true). When definite is false, chance
// is the percent chance (already capped at 50) to compare against a roll.
func ChestUnlockRate(level, skillLevel int) (chance int, definite bool, succeeds bool) {
	switch {
	case level > 60:
		if skillLevel < 10 {
			return 0, true, false
		}
		chance = (skillLevel-10)*5 + 30
	case level > 40:
		if skillLevel < 6 {
			return 0, true, false
		}
		chance = (skillLevel-6)*5 + 10
	case level > 30:
		if skillLevel < 3 {
			return 0, true, false
		}
		if skillLevel > 12 {
			return 0, true, true
		}
		chance = (skillLevel-3)*5 + 30
	default:
		if skillLevel > 10 {
			return 0, true, true
		}
		chance = skillLevel*5 + 35
	}

	if chance > 50 {
		chance = 50
	}
	return chance, false, false
}
