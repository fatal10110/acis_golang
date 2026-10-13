package db

import "testing"

// The expected column values were captured by writing the same UTF-16
// strings through the bundled MariaDB connector and reading back HEX(v).
func TestStoredTextReplacesUnpairedSurrogates(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain text", "Player", "Player"},
		{"supplementary character", "x\U0001F600y", "x\U0001F600y"},
		{"highest unit before surrogates", "\xED\x9F\xBF", "\xED\x9F\xBF"},
		{"lone high mid-string", "a\xED\xA0\xBDb", "a?b"},
		{"lone low", "a\xED\xB8\x80b", "a?b"},
		{"lone high ending the string", "ab\xED\xA0\xBD", "abc"},
		{"only a lone low", "\xED\xB8\x80", "?"},
		{"low then high", "\xED\xB8\x80\xED\xA0\xBD", "?c"},
		{"high before a pair", "\xED\xA0\xBD\U0001F600", "?\U0001F600"},
		{"halves spelled apart", "\xED\xA0\xBD\xED\xB8\x80", "\U0001F600"},
	}
	for _, tc := range cases {
		if got := StoredText(tc.in); got != tc.want {
			t.Errorf("%s: StoredText(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}
