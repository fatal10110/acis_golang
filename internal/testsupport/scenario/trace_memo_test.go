package scenario

import (
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// memoTrace is a scenario trace of two kills: the first sets a player memo
// (and reads the memos back, which is not a write), the second deletes it.
const memoTrace = `# test trace
# player object 500; format: README.md in this directory.
# kill 20120 (object 700)
S ActionFailed 25
Q INSERT INTO character_memo (charId, var, val) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE val = VALUES(val) | 500 | Q260_OneTimeQuestFlag | true
Q SELECT * FROM character_memo WHERE charId = ? | 500
# kill 20120 (object 701)
S ActionFailed 25
Q DELETE FROM character_memo WHERE charId = ? AND var = ? | 500 | Q260_OneTimeQuestFlag
`

// A traced step's memo writes must be the trace's, in order: one too few,
// one too many, another value, another step or another write fails.
func TestCheckTraceComparesMemoWrites(t *testing.T) {
	const (
		set   = "  M set Q260_OneTimeQuestFlag | true\n"
		unset = "  M unset Q260_OneTimeQuestFlag\n"
	)
	for _, tc := range []struct{ name, steps, want string }{
		{"replays", "kill wolf\n  S ActionFailed\n" + set + "kill wolf\n  S ActionFailed\n" + unset, ""},
		{"one too few", "kill wolf\n  S ActionFailed\nkill wolf\n  S ActionFailed\n" + unset, `step 1 (line 4) "kill wolf" against trace "kill 20120": scenario memo 1 missing:` + "\n want set Q260_OneTimeQuestFlag | true"},
		{"one too many", "kill wolf\n  S ActionFailed\n" + set + "  M set Q211_Flag | 1\nkill wolf\n  S ActionFailed\n" + unset, "scenario memo 2 not expected:\n  got set Q211_Flag | 1"},
		{"other value", "kill wolf\n  S ActionFailed\n  M set Q260_OneTimeQuestFlag | false\nkill wolf\n  S ActionFailed\n" + unset, "scenario memo 1 differs:\n  got set Q260_OneTimeQuestFlag | false\n want set Q260_OneTimeQuestFlag | true"},
		{"other step", "kill wolf\n  S ActionFailed\nkill wolf\n  S ActionFailed\n" + set + unset, "scenario memo 1 missing"},
		{"set for unset", "kill wolf\n  S ActionFailed\n" + set + "kill wolf\n  S ActionFailed\n" + set, `step 2 (line 7) "kill wolf" against trace "kill 20120": scenario memo 1 differs`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := checkTrace(traceScenario(t, tc.steps), []byte(memoTrace), nil)
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("checkTrace error = %v, want none", err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Fatalf("checkTrace error = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

// A memo statement other than the set and the unset fails the trace, as an
// unknown journal statement does.
func TestCheckTraceRejectsAnUnknownMemoStatement(t *testing.T) {
	trace := "# player object 500\n# kill 20120 (object 700)\nQ DELETE FROM character_memo WHERE charId=? | 500\n"
	_, _, err := checkTrace(traceScenario(t, "kill wolf\n"), []byte(trace), nil)
	if want := `is not a memo write`; err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("checkTrace error = %v, want one containing %q", err, want)
	}
}

func TestParseReadsMemoLines(t *testing.T) {
	sc := traceScenario(t, "kill wolf\n  M set a | b c\n  M unset a\n")
	if got, want := sc.Steps[0].Memos, []string{"set a | b c", "unset a"}; !slices.Equal(got, want) {
		t.Fatalf("memos = %q, want %q", got, want)
	}
	for _, bad := range []string{"M a | b", "M set a", "M unset", "M unset a b"} {
		if _, err := Parse("bad.scenario", []byte("character A 1\nadvance 1s\n  "+bad+"\n")); err == nil {
			t.Errorf("Parse accepted %q", bad)
		}
	}
}

// The run renders its memo writes as M lines.
func TestMemoLinesRenderWrites(t *testing.T) {
	got := memoLines([]gameservertest.MemoWrite{{Key: "a", Value: "1"}, {Key: "b", Unset: true}})
	if want := []string{"set a | 1", "unset b"}; !slices.Equal(got, want) {
		t.Fatalf("memoLines = %q, want %q", got, want)
	}
}
