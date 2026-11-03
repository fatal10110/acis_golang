package scenario

import (
	"slices"
	"strings"
	"testing"
)

// stepsTrace is a scenario trace: two kills, a hit, an advance with a timer
// firing, a pinned draw and a player memo write between them.
const stepsTrace = `# test trace
# player object 500; format: README.md in this directory.
# kill 20120 (object 700)
S ActionFailed 25
T 500
# draw 1000000:0 Quest
Q INSERT INTO character_memo (charId, var, val) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE val = VALUES(val) | 500 | Q260_OneTimeQuestFlag | true
Q INSERT INTO character_quests (charId,name,var,value) VALUES (?,?,?,?) ON DUPLICATE KEY UPDATE value=VALUES(value) | 500 | Q001_LettersOfLove | <cond> | 2
# kill 20120 (object 701)
S ActionFailed 25
# hit 20120 (object 702) 10
S ActionFailed 25
T 500
# advance 1500
T 500
T 500
T 500
S ActionFailed 25
`

// A kill step of N kills replays N kills of the trace, hits and advances
// replay theirs, and a player memo write is an M line, not a Q line.
func TestCheckTraceReplaysKillsHitsAndAdvances(t *testing.T) {
	sc := traceScenario(t, "random always 0\nkill wolf x2\n  S ActionFailed\n  S ActionFailed\n  Q upsert Q001_LettersOfLove | <cond> | 2\n  M set Q260_OneTimeQuestFlag | true\nhit wolf 10\n  S ActionFailed\nadvance 1500ms\n  S ActionFailed\nrelog\n")
	draws, _, err := checkTrace(sc, []byte(stepsTrace), nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := [][]string{{"1000000:0"}, nil, nil}; !slices.EqualFunc(draws, want, slices.Equal) {
		t.Fatalf("trace draws = %q, want %q", draws, want)
	}
}

func TestCheckTraceNamesADepartureOfAKillHitOrAdvance(t *testing.T) {
	for _, tc := range []struct{ name, steps, want string }{
		{"fewer kills", "kill wolf\n  S ActionFailed\n  Q upsert Q001_LettersOfLove | <cond> | 2\n  M set Q260_OneTimeQuestFlag | true\nhit wolf 10\n", `step 2 (line 8) "hit wolf 10" against trace "kill 20120": the step sends "hit 20120 10"`},
		{"other damage", "kill wolf x2\n  S ActionFailed\n  S ActionFailed\n  Q upsert Q001_LettersOfLove | <cond> | 2\n  M set Q260_OneTimeQuestFlag | true\nhit wolf 11\n", `the step sends "hit 20120 11"`},
		{"other time", "kill wolf x2\n  S ActionFailed\n  S ActionFailed\n  Q upsert Q001_LettersOfLove | <cond> | 2\n  M set Q260_OneTimeQuestFlag | true\nhit wolf 10\n  S ActionFailed\nadvance 1s\n", `the step sends "advance 1000"`},
		{"missing timer packet", "kill wolf x2\n  S ActionFailed\n  S ActionFailed\n  Q upsert Q001_LettersOfLove | <cond> | 2\n  M set Q260_OneTimeQuestFlag | true\nhit wolf 10\n  S ActionFailed\nadvance 1500ms\n", "scenario packet 1 missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := checkTrace(traceScenario(t, tc.steps), []byte(stepsTrace), nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("checkTrace error = %v, want one containing %q", err, tc.want)
			}
		})
	}
}
