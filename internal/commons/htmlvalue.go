package commons

import "strings"

// HTMLValue is value as an HTML window's placeholder substitution writes
// it into the page. The substitution reads a backslash as quoting the
// character after it, which it keeps alone, so "a\b" shows as "ab" and
// "a\\b" as "a\b". A backslash ending the value is kept, where the
// reference fails and sends no page at all.
func HTMLValue(value string) string {
	if !strings.Contains(value, `\`) {
		return value
	}
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+1 < len(value) {
			i++
		}
		b.WriteByte(value[i])
	}
	return b.String()
}
