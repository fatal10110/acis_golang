package bbs

import (
	"strings"
	"testing"
)

// TestNoticeTextCarriesNoLink pins the clan notice hardening (#3214): a
// link word nested in another, or split by a backslash the window drops,
// is taken out with the rest, so the notice shows no link at all.
func TestNoticeTextCarriesNoLink(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Raid at 9\r\n<a action=\"bypass -h x\">go</a>", `Raid at 9<br><a =" -h x">go</a>`},
		{`<a acactiontion="bypbypassass -h npc_1_x">go</a>`, `<a =" -h npc_1_x">go</a>`},
		{`<a a\ction="b\ypass -h npc_1_x">go</a>`, `<a =" -h npc_1_x">go</a>`},
		{`<a ac\actiontion="byp\bypassass -h x">go</a>`, `<a =" -h x">go</a>`},
		{`a\\b end\`, `a\b end\`},
	} {
		got := NoticeText(tc.in)
		if got != tc.want {
			t.Errorf("NoticeText(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if strings.Contains(got, "action") || strings.Contains(got, "bypass") {
			t.Errorf("NoticeText(%q) = %q still carries a link word", tc.in, got)
		}
	}
}
