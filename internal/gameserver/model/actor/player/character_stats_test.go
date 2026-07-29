package player

import (
	"math"
)

func closeFloat(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
