package commons

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// DecodeInt32 parses s as a data-file integer literal: an optional sign,
// then a decimal number, a hexadecimal number after "0x", "0X" or "#", or an
// octal number after a leading "0" ("010" is 8). Digits are those of
// ParseInt, plus fullwidth Latin letters as hexadecimal digits; the sign and
// the base prefix are ASCII only. Nothing else is accepted:
// no surrounding space, no digit separators, no "0b"/"0o" prefixes, and no
// second sign after the base prefix. The value must fit int32; the sign
// applies before the range check, so "-0x80000000" is math.MinInt32.
func DecodeInt32(s string) (int32, error) {
	rest := s
	neg := false
	if rest != "" && (rest[0] == '-' || rest[0] == '+') {
		neg = rest[0] == '-'
		rest = rest[1:]
	}

	base := 10
	switch {
	case strings.HasPrefix(rest, "0x"), strings.HasPrefix(rest, "0X"):
		base, rest = 16, rest[2:]
	case strings.HasPrefix(rest, "#"):
		base, rest = 16, rest[1:]
	case len(rest) > 1 && rest[0] == '0':
		base, rest = 8, rest[1:]
	}
	// An explicit base keeps strconv from reading prefixes or "_" itself; a
	// sign is rejected here because strconv would otherwise accept one.
	if rest == "" || rest[0] == '-' || rest[0] == '+' {
		return 0, fmt.Errorf("%q: invalid integer literal", s)
	}
	n, err := strconv.ParseUint(asciiDigits(rest), base, 64)
	if err != nil {
		return 0, fmt.Errorf("%q: invalid integer literal", s)
	}
	if neg {
		if n > -math.MinInt32 {
			return 0, fmt.Errorf("%q: value overflows int32", s)
		}
		return int32(-int64(n)), nil
	}
	if n > math.MaxInt32 {
		return 0, fmt.Errorf("%q: value overflows int32", s)
	}
	return int32(n), nil
}

// ParseInt is strconv.ParseInt(s, 10, bitSize) over every digit a data file
// may use: besides '0'-'9', any Unicode decimal digit in the Basic
// Multilingual Plane (Arabic-Indic, Devanagari, fullwidth and the like) reads
// as its value. The sign is still an ASCII '+' or '-', and every other rule
// of strconv.ParseInt holds. An error is a *strconv.NumError naming s.
func ParseInt(s string, bitSize int) (int64, error) {
	n, err := strconv.ParseInt(asciiDigits(s), 10, bitSize)
	return n, numErrorFor(err, s)
}

// Atoi is strconv.Atoi over the digits ParseInt reads.
func Atoi(s string) (int, error) {
	n, err := strconv.Atoi(asciiDigits(s))
	return n, numErrorFor(err, s)
}

// numErrorFor reports a strconv error against the caller's input rather
// than its ASCII form.
func numErrorFor(err error, s string) error {
	if ne, ok := errors.AsType[*strconv.NumError](err); ok {
		ne.Num = s
	}
	return err
}

// asciiDigits returns s with each non-ASCII character that reads as a digit
// replaced by its ASCII form: a Unicode decimal digit (category Nd) in the
// Basic Multilingual Plane becomes '0'-'9', and a fullwidth Latin letter
// becomes 'a'-'z', a digit only in a base above ten. Everything else,
// including digits outside that plane, is kept for the ASCII parser to
// reject.
func asciiDigits(s string) string {
	i := 0
	for i < len(s) && s[i] < utf8.RuneSelf {
		i++
	}
	if i == len(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	b.WriteString(s[:i])
	for _, r := range s[i:] {
		if d, ok := asciiDigit(r); ok {
			b.WriteByte(d)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// asciiDigit maps one non-ASCII digit character to its ASCII form. Decimal
// digits come in runs of ten from zero to nine, so a run's offset gives the
// value.
func asciiDigit(r rune) (byte, bool) {
	switch {
	case r < utf8.RuneSelf || r > 0xFFFF:
		return 0, false
	case r >= 0xFF21 && r <= 0xFF3A:
		return byte('a' + r - 0xFF21), true
	case r >= 0xFF41 && r <= 0xFF5A:
		return byte('a' + r - 0xFF41), true
	}
	for _, rg := range unicode.Nd.R16 {
		if r < rune(rg.Lo) {
			break
		}
		if r <= rune(rg.Hi) {
			return byte('0' + (r-rune(rg.Lo))%10), true
		}
	}
	return 0, false
}
