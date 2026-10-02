package commons

import "math"

// JavaInt narrows f to an int32 by truncating toward zero, saturating at the
// int32 range, with NaN as 0.
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

// JavaLong narrows f to an int64 by truncating toward zero, saturating at
// the int64 range, with NaN as 0.
func JavaLong(f float64) int64 {
	switch {
	case math.IsNaN(f):
		return 0
	case f >= math.MaxInt64:
		return math.MaxInt64
	case f <= math.MinInt64:
		return math.MinInt64
	}
	return int64(f)
}

// JavaRound rounds f half up to an int64, saturating at the int64 range,
// with NaN as 0.
func JavaRound(f float64) int64 {
	floor := math.Floor(f)
	if f-floor >= 0.5 {
		floor++
	}
	return JavaLong(floor)
}
