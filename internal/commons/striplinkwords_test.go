package commons

import "testing"

// TestStripLinkWords pins the link words taken out until none is left,
// including the ones a single pass would leave behind.
func TestStripLinkWords(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ in, want string }{
		{"", ""},
		{"plain text", "plain text"},
		{`<a action="bypass -h x">go</a>`, `<a =" -h x">go</a>`},
		{"acactiontion", ""},
		{"bypbypassass", ""},
		{"actbypassion", ""},
		{"bypactionass", ""},
		{"acacactiontiontion bypbypbypassassass", " "},
		{"Action Bypass", "Action Bypass"},
	} {
		if got := StripLinkWords(tc.in); got != tc.want {
			t.Errorf("StripLinkWords(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
