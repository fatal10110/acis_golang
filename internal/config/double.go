package config

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

// parseDouble parses a double property value. After dropping leading and
// trailing ASCII control characters and spaces it accepts an optional sign
// followed by one of:
//   - a decimal literal: digits with an optional point (at least one digit
//     in total) and an optional e/E exponent with its own optional sign;
//   - a hexadecimal literal: 0x or 0X, hex digits with an optional point (at
//     least one digit in total) and a mandatory p/P binary exponent;
//   - exactly "Infinity" or "NaN".
//
// A decimal or hexadecimal literal may end in one d, D, f or F type suffix,
// which does not change the value. A finite literal beyond the float64 range
// yields a signed infinity rather than an error. Anything else, including
// "inf" spellings and digit separators, is a syntax error.
func parseDouble(s string) (float64, error) {
	body := strings.TrimFunc(s, func(r rune) bool { return r <= ' ' })
	sign := ""
	if body != "" && (body[0] == '+' || body[0] == '-') {
		sign, body = body[:1], body[1:]
	}
	switch body {
	case "NaN":
		return math.NaN(), nil
	case "Infinity":
		if sign == "-" {
			return math.Inf(-1), nil
		}
		return math.Inf(1), nil
	}
	if n := len(body); n > 0 {
		switch body[n-1] {
		case 'd', 'D', 'f', 'F':
			body = body[:n-1]
		}
	}
	var ok bool
	if len(body) > 1 && body[0] == '0' && (body[1] == 'x' || body[1] == 'X') {
		ok = validHexDouble(body[2:])
	} else {
		ok = validDecimalDouble(body)
	}
	if !ok {
		return 0, &strconv.NumError{Func: "ParseFloat", Num: s, Err: strconv.ErrSyntax}
	}
	v, err := strconv.ParseFloat(sign+body, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return 0, err
	}
	// Overflow already reports ±Inf; that is the value, not a failure.
	return v, nil
}

// validDecimalDouble reports whether b is digits with an optional point, at
// least one digit in total, then an optional exponent.
func validDecimalDouble(b string) bool {
	mant, ok := mantissaLen(b, isDecimalDigit)
	if !ok {
		return false
	}
	rest := b[mant:]
	if rest == "" {
		return true
	}
	if rest[0] != 'e' && rest[0] != 'E' {
		return false
	}
	return validExponent(rest[1:])
}

// validHexDouble reports whether b (after the 0x prefix) is hex digits with
// an optional point, at least one digit in total, then a binary exponent.
func validHexDouble(b string) bool {
	mant, ok := mantissaLen(b, isHexDigit)
	if !ok {
		return false
	}
	rest := b[mant:]
	if rest == "" || (rest[0] != 'p' && rest[0] != 'P') {
		return false
	}
	return validExponent(rest[1:])
}

// mantissaLen returns the length of the leading digits-and-point run of b
// and whether that run holds at most one point and at least one digit.
func mantissaLen(b string, digit func(byte) bool) (int, bool) {
	i, digits, point := 0, 0, false
	for ; i < len(b); i++ {
		switch {
		case digit(b[i]):
			digits++
		case b[i] == '.' && !point:
			point = true
		default:
			return i, digits > 0
		}
	}
	return i, digits > 0
}

// validExponent reports whether b is an optional sign then one or more
// decimal digits.
func validExponent(b string) bool {
	if b != "" && (b[0] == '+' || b[0] == '-') {
		b = b[1:]
	}
	if b == "" {
		return false
	}
	for i := range len(b) {
		if !isDecimalDigit(b[i]) {
			return false
		}
	}
	return true
}

func isDecimalDigit(c byte) bool { return c >= '0' && c <= '9' }

func isHexDigit(c byte) bool {
	return isDecimalDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
