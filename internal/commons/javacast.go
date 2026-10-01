package commons

import "math"

// JavaInt narrows f to an int32 the way a Java (int) cast does: toward
// zero, saturating at the int32 range, NaN as 0.
func JavaInt(f float64) int32 {
	switch {
	case math.IsNaN(f):
		return 0
	case f >= math.MaxInt32:
		return math.MaxInt32
	case f <= math.MinInt32:
		return math.MinInt32
	}
	return int32(f)
}
