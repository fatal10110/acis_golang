package commons

import (
	"math"
	"strconv"
	"strings"
)

// JavaFixed formats f with decimals digits after the point the way the
// reference's String.format("%.<decimals>f") does: the shortest decimal
// that reads back as f, rounded half up. A tie in that decimal rounds away
// from zero even where the binary value sits just below it, so 1.25 reads
// "1.3" with one digit where Go's %.1f reads "1.2".
func JavaFixed(f float64, decimals int) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	}
	if decimals < 0 {
		decimals = 0
	}
	sign := ""
	if math.Signbit(f) {
		sign = "-"
	}
	shortest := strconv.FormatFloat(math.Abs(f), 'f', -1, 64)
	whole, frac, _ := strings.Cut(shortest, ".")
	if len(frac) <= decimals {
		frac += strings.Repeat("0", decimals-len(frac))
		return joinFixed(sign, whole, frac)
	}
	up := frac[decimals] >= '5'
	digits := []byte(whole + frac[:decimals])
	if up {
		i := len(digits) - 1
		for ; i >= 0 && digits[i] == '9'; i-- {
			digits[i] = '0'
		}
		if i < 0 {
			digits = append([]byte{'1'}, digits...)
		} else {
			digits[i]++
		}
	}
	cut := len(digits) - decimals
	return joinFixed(sign, string(digits[:cut]), string(digits[cut:]))
}

func joinFixed(sign, whole, frac string) string {
	if frac == "" {
		return sign + whole
	}
	return sign + whole + "." + frac
}
