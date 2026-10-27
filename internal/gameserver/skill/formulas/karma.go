package formulas

import "github.com/fatal10110/acis_golang/internal/commons"

// pkKarmaPlateau is the karma a PK kill awards once the killer's PK count
// reaches 180.
const pkKarmaPlateau = 14400

// KarmaGain returns the karma a PK kill awards the killer, keyed off its PK
// count. Gain ramps linearly below 100 kills, ramps more slowly from 100 up
// to 180, then plateaus. Killing a summon awards about a quarter of that,
// with pkCount's two low bits added before the division.
func KarmaGain(pkCount int, summon bool) int {
	result := pkKarmaPlateau
	switch {
	case pkCount < 100:
		result = int((((float64(pkCount-1) * 0.5) + 1) * 60) * 4)
	case pkCount < 180:
		result = int((((float64(pkCount+1) * 0.125) + 37.5) * 60) * 4)
	}
	if summon {
		result = ((pkCount & 3) + result) >> 2
	}
	return result
}

// KarmaLoss returns the karma that exp points of experience work off at a
// level with the given karma modifier, narrowed like an int32 cast.
func KarmaLoss(exp int64, modifier float64) int {
	return int(commons.TruncInt32Sat(float64(exp) / modifier / 15))
}
