package clan

import "testing"

// TestValidTitleMatchesReferencePattern pins ValidTitle to the reference's
// title pattern, ^[a-zA-Z0-9 !@#$&()\-`.+,/"]*{0,16}$ under
// Matcher.matches(). The expectations were observed by compiling and
// running that pattern on a JDK 21: it compiles, and the {0,16} after the
// star applies to an empty atom, so it bounds nothing.
func TestValidTitleMatchesReferencePattern(t *testing.T) {
	for title, want := range map[string]bool{
		"":                        true,
		"abc":                     true,
		"Hello World":             true,
		"12345678901234567890123": true,
		"!@#$&()-`.+,/\"":         true,
		"a_b":                     false,
		"a*b":                     false,
		"a%b":                     false,
		"ä":                       false,
		"a'b":                     false,
		"a<b":                     false,
		"a=b":                     false,
		"a?b":                     false,
		"a~b":                     false,
		"a[b":                     false,
		"a:b":                     false,
		"a;b":                     false,
		"a\\b":                    false,
		"a^b":                     false,
		"a{b":                     false,
		"a|b":                     false,
		"a\tb":                    false,
		"a\nb":                    false,
		"ab\n":                    false,
	} {
		if got := ValidTitle(title); got != want {
			t.Errorf("ValidTitle(%q) = %v, want %v", title, got, want)
		}
	}
}
