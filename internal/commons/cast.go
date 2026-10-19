package commons

import "math"

// TruncInt32Sat narrows f to an int32 by truncating toward zero, saturating
// at the int32 range, with NaN as 0.
func TruncInt32Sat(f float64) int32 {
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

// TruncInt64Sat narrows f to an int64 by truncating toward zero, saturating at
// the int64 range, with NaN as 0.
func TruncInt64Sat(f float64) int64 {
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

// RoundHalfUp rounds f half up to an int64, saturating at the int64 range,
// with NaN as 0.
func RoundHalfUp(f float64) int64 {
	floor := math.Floor(f)
	if f-floor >= 0.5 {
		floor++
	}
	return TruncInt64Sat(floor)
}
