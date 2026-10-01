package commons

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// DecodeInt32 parses s as a data-file integer literal: an optional sign,
// then a decimal number, a hexadecimal number after "0x", "0X" or "#", or an
// octal number after a leading "0" ("010" is 8). Nothing else is accepted:
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
	n, err := strconv.ParseUint(rest, base, 64)
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
