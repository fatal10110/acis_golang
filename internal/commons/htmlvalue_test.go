package commons

import "testing"

// TestHTMLValue pins NpcHtmlMessage.replace's backslash handling: each
// backslash quotes the next character, a quoted backslash stays, and a
// dollar sign is plain text. A quoted dollar sign still sends the page,
// kept as a parity exception (#4718).
func TestHTMLValue(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ in, want string }{
		{"", ""},
		{"plain $5 text", "plain $5 text"},
		{`a\b`, "ab"},
		{`a\\b`, `a\b`},
		{`\\\\`, `\\`},
		{`\$1`, "$1"},
		{`a\$`, "a$"},
		{`a\$b`, "a$b"},
		{`a\\\$1`, `a\$1`},
		{`end\`, `end\`},
		{`end\\\`, `end\\`},
	} {
		if got := HTMLValue(tc.in); got != tc.want {
			t.Errorf("HTMLValue(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
