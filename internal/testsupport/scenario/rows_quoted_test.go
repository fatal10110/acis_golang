package scenario

import (
	"slices"
	"strings"
	"testing"
)

func TestSplitQuotedKeepsBlanksInsideQuotes(t *testing.T) {
	for _, tc := range []struct {
		line string
		want []string
	}{
		{`Q419 quiz="20 21 22" correct=3`, []string{"Q419", "quiz=20 21 22", "correct=3"}},
		{`Q419   quiz=""  x=1`, []string{"Q419", "quiz=", "x=1"}},
		{"Q419\tquiz=\"a  b\"", []string{"Q419", "quiz=a  b"}},
		{`a=1 b="x"y`, []string{"a=1", "b=xy"}},
		{`  `, nil},
	} {
		got, err := splitQuoted(tc.line)
		if err != nil || !slices.Equal(got, tc.want) {
			t.Errorf("splitQuoted(%q) = %q, %v, want %q", tc.line, got, err, tc.want)
		}
	}
	if _, err := splitQuoted(`Q quiz="20 21`); err == nil || !strings.Contains(err.Error(), "unclosed quote") {
		t.Errorf("unclosed quote error = %v", err)
	}
}

// TestParseReadsAQuotedValueInRowsAndQuestLines: a quest variable holding a
// list (Q419's quiz) is one <var>=<value> field once quoted.
func TestParseReadsAQuotedValueInRowsAndQuestLines(t *testing.T) {
	sc, err := Parse("quoted.scenario", []byte(`character A 1
quest Q419 quiz="20 21 22" <state>=STARTED
relog
  rows Q419 quiz="20 21" <state>=STARTED
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := sc.Quests[0].Vars; len(got) != 2 || got[0] != [2]string{"quiz", "20 21 22"} {
		t.Fatalf("quest vars = %q", got)
	}
	ck := sc.Steps[0].Checks[0]
	if want := []string{"Q419", "quiz=20 21", "<state>=STARTED"}; !slices.Equal(ck.Args, want) {
		t.Fatalf("rows args = %q, want %q", ck.Args, want)
	}
	if _, err := Parse("bad.scenario", []byte("character A 1\nrelog\n  rows Q419 quiz=\"20\n")); err == nil || !strings.Contains(err.Error(), "unclosed quote") {
		t.Fatalf("unclosed quote error = %v", err)
	}
}
