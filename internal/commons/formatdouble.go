package commons

import (
	"math"
	"strconv"
	"strings"
)

// FormatDouble writes v as a double's decimal string: the shortest decimal that
// reads back as v, with at least one digit after the point, in scientific form
// ("1.0E7") outside [0.001, 10000000).
func FormatDouble(v float64) string {
	return formatFloat(v, 64)
}

// FormatFloat writes v as a float's decimal string, the way FormatDouble
// writes a double: the shortest decimal that reads back as the float v.
func FormatFloat(v float32) string {
	return formatFloat(float64(v), 32)
}

func formatFloat(v float64, bits int) string {
	switch {
	case math.IsNaN(v):
		return "NaN"
	case math.IsInf(v, 1):
		return "Infinity"
	case math.IsInf(v, -1):
		return "-Infinity"
	}
	if a := math.Abs(v); a == 0 || (a >= 1e-3 && a < 1e7) {
		s := strconv.FormatFloat(v, 'f', -1, bits)
		if !strings.Contains(s, ".") {
			s += ".0"
		}
		return s
	}
	mantissa, exp, _ := strings.Cut(strconv.FormatFloat(v, 'E', -1, bits), "E")
	if !strings.Contains(mantissa, ".") {
		mantissa += ".0"
	}
	e, _ := strconv.Atoi(exp)
	return mantissa + "E" + strconv.Itoa(e)
}
