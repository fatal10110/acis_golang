package commons

import "testing"

func TestHTMLText(t *testing.T) {
	for in, want := range map[string]string{
		"plain words":                 "plain words",
		`<a action="bypass x">go</a>`: "&lt;a action=&quot;bypass x&quot;&gt;go&lt;/a&gt;",
		"a & b's":                     "a & b's",
		"&lt;":                        "&lt;",
		"":                            "",
	} {
		if got := HTMLText(in); got != want {
			t.Errorf("HTMLText(%q) = %q, want %q", in, got, want)
		}
	}
}
