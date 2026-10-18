package commons

import "strconv"

// FormatNumber writes n in decimal with a comma between each group of
// three digits, as an English number format does: 1234567 reads
// "1,234,567".
func FormatNumber(n int64) string {
	s := strconv.FormatInt(n, 10)
	sign := ""
	if n < 0 {
		sign, s = "-", s[1:]
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return sign + s
}
