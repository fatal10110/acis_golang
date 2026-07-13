package player

import "math"

// killLevelPenaltyThreshold is how many levels an attacker can outlevel its
// kill before the exp/sp reward starts falling off.
const killLevelPenaltyThreshold = 5

// killLevelPenaltyBase is the per-extra-level decay factor applied once the
// attacker exceeds killLevelPenaltyThreshold.
const killLevelPenaltyBase = 5.0 / 6.0

// KillRewardExpAndSp returns one attacker's exp and sp share of a kill.
//
// expReward and spReward are the victim's full reward at the current server
// rate; damage is this attacker's share of totalDamage, the combined damage
// every rewarded attacker dealt. Both are split proportionally to that
// share: a totalDamage of zero or less yields no reward.
//
// levelDiff is the attacker's level minus the victim's. Once it exceeds
// killLevelPenaltyThreshold, both exp and sp are scaled down by
// killLevelPenaltyBase raised to the number of levels past the threshold —
// unlike the drop-rate level penalty, this falloff has no floor, so a large
// enough gap yields zero reward. A non-positive resulting exp also zeroes
// sp, matching the reward always granting sp alongside a nonzero exp only.
//
// The returned values are narrowed through the legacy signed 32-bit reward
// contract before being widened to this package's public return types.
func KillRewardExpAndSp(expReward, spReward float64, damage, totalDamage float64, levelDiff int) (exp int64, sp int) {
	if totalDamage <= 0 {
		return 0, 0
	}

	xp := expReward * damage / totalDamage
	spF := spReward * damage / totalDamage

	if levelDiff > killLevelPenaltyThreshold {
		falloff := math.Pow(killLevelPenaltyBase, float64(levelDiff-killLevelPenaltyThreshold))
		xp *= falloff
		spF *= falloff
	}

	if xp <= 0 {
		return 0, 0
	}
	if spF <= 0 {
		spF = 0
	}
	return int64(rewardInt32(xp)), int(rewardInt32(spF))
}

func rewardInt32(v float64) int32 {
	if math.IsNaN(v) {
		return 0
	}
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
}
