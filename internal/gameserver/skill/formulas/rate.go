package formulas

import "math"

// clampInt restricts n to [lo, hi].
func clampInt(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

// HitRate returns the per-mille (0-1000) value an attack roll is compared
// against to decide hit-vs-miss: higher means more likely to hit. accuracy
// and evasion are the attacker's/defender's already-computed combat stats;
// diffZ is the attacker's Z minus the defender's Z; night, behind and
// inFront describe the attacker's circumstances relative to the target.
// The result is clamped to [300, 980] — even a huge accuracy advantage
// can't guarantee a hit, and even a huge disadvantage can't guarantee a
// miss.
func HitRate(accuracy, evasion, diffZ int, night, behind, inFront bool) int {
	diff := accuracy - evasion

	if diffZ > 50 {
		diff += 3
	} else if diffZ < -50 {
		diff -= 3
	}

	if night {
		diff -= 10
	}

	if behind {
		diff += 10
	} else if !inFront {
		diff += 5
	}

	return clampInt((90+2*diff)*10, 300, 980)
}

// Missed reports whether an attack with the given HitRate misses, given
// roll, a uniform random draw in [0, 1000).
func Missed(rate, roll int) bool {
	return rate < roll
}

// PosMul returns the positional damage multiplier for an attack landed
// from behind, from the side/front, or (with crit true) reduced for a
// critical hit's already-large base multiplier.
func PosMul(behind, inFront, crit bool) float64 {
	if behind {
		if crit {
			return 1.1
		}
		return 1.2
	}
	if !inFront {
		if crit {
			return 1.025
		}
		return 1.05
	}
	return 1.
}

// CritSucceeds reports whether a critical-hit roll succeeds, given rate
// (the attacker's already-computed critical-rate stat, per-mille) and roll,
// a uniform random draw in [0, 1000).
func CritSucceeds(rate float64, roll int) bool {
	return rate > float64(roll)
}

// MCritSucceeds reports whether a magic-critical-hit roll succeeds, given
// mRate (the attacker's already-computed magic-critical-rate stat,
// per-mille) and roll, a uniform random draw in [0, 1000).
func MCritSucceeds(mRate, roll int) bool {
	return mRate > roll
}

// MagicSuccessRate returns the per-10000 chance (clamped to at most 9900)
// that a target resists a caster's magic skill: targetLevel and
// casterLevel are the two creatures' levels; magicLevel is the skill's own
// magic level (0 to fall back to casterLevel); levelDepend is the skill's
// configured level-dependency bonus; weaponGradePenalty reports whether
// the caster's weapon grade is insufficient for the skill (a flat +6000
// penalty).
func MagicSuccessRate(targetLevel, casterLevel, magicLevel, levelDepend int, weaponGradePenalty bool) float64 {
	base := magicLevel
	if base <= 0 {
		base = casterLevel
	}

	lvlDifference := targetLevel - (base + levelDepend)
	rate := 100.0
	if lvlDifference > 0 {
		rate = math.Pow(1.166, float64(lvlDifference)) * 100
	}

	if weaponGradePenalty {
		rate += 6000
	}

	return math.Min(rate, 9900)
}

// MagicSucceeds reports whether the caster's magic skill succeeds (i.e.
// the target does NOT resist), given rate (from MagicSuccessRate) and
// roll, a uniform random draw in [0, 10000).
func MagicSucceeds(rate float64, roll int) bool {
	return float64(roll) > rate
}
