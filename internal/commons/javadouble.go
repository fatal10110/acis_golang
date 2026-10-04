package commons

import (
	"math"
	"strconv"
	"strings"
)

// JavaDouble writes v the way the reference's String.valueOf(double) does:
// the shortest decimal that reads back as v, with at least one digit after
// the point, in scientific form ("1.0E7") outside [0.001, 10000000).
func JavaDouble(v float64) string {
	switch {
	case math.IsNaN(v):
		return "NaN"
	case math.IsInf(v, 1):
		return "Infinity"
	case math.IsInf(v, -1):
		return "-Infinity"
	}
	if a := math.Abs(v); a == 0 || (a >= 1e-3 && a < 1e7) {
		s := strconv.FormatFloat(v, 'f', -1, 64)
		if !strings.Contains(s, ".") {
			s += ".0"
		}
		return s
	}
	mantissa, exp, _ := strings.Cut(strconv.FormatFloat(v, 'E', -1, 64), "E")
	if !strings.Contains(mantissa, ".") {
		mantissa += ".0"
	}
	e, _ := strconv.Atoi(exp)
	return mantissa + "E" + strconv.Itoa(e)
}
