package scenario

import (
	"strings"
	"testing"
)

// drawsTrace is a scenario trace of two kills: the first draws twice, the
// second once.
const drawsTrace = `# test trace
# player object 500; format: README.md in this directory.
# kill 20120 (object 700)
# draw 100:0 Q211_TrialOfTheChallenger
# draw 1000000:0 Quest
S ActionFailed 25
# kill 20120 (object 701)
# draw 1000000:0 Quest
S ActionFailed 25
`

// A journal statement the format does not know fails the trace, while
// another table's statement is skipped.
func TestCheckTraceRejectsAnUnknownJournalStatement(t *testing.T) {
	for _, tc := range []struct{ name, stmt, want string }{
		{"unknown journal statement", "Q DELETE FROM character_quests WHERE charId=? | 500", `statement "DELETE FROM character_quests WHERE charId=?" is not a journal statement`},
		{"other table", "Q INSERT INTO character_raid_points (char_id, boss_id, points) VALUES (?,?,?) | 500 | 1 | 2", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			trace := "# player object 500\n# kill 20120 (object 700)\nS ActionFailed 25\n" + tc.stmt + "\n"
			_, _, err := checkTrace(traceScenario(t, "kill wolf\n  S ActionFailed\n"), []byte(trace), nil)
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("checkTrace error = %v, want none", err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Fatalf("checkTrace error = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

// The run compares each traced step's script draws with the trace's: the
// same bounds, in the same order and count.
func TestTraceDrawsCompareBoundsPerStep(t *testing.T) {
	sc := traceScenario(t, "random always 0\nkill wolf\n  S ActionFailed\nkill wolf\n  S ActionFailed\nkill wolf\n")
	want, _, err := checkTrace(sc, []byte(drawsTrace), nil)
	if err != nil {
		t.Fatal(err)
	}
	// play draws what a Go step would, through the run's random source.
	play := func(bounds ...int) []string {
		d := &draws{random: sc.Random}
		for _, n := range bounds {
			d.draw(n)
		}
		drawn, bad := d.take()
		if len(bad) > 0 {
			t.Fatalf("bad draws %v", bad)
		}
		return drawn
	}
	if d := traceDrawDiff(want, 0, play(100, 1000000)); d != "" {
		t.Fatalf("matching draws: %s", d)
	}
	if d := traceDrawDiff(want, 1, play(1000000)); d != "" {
		t.Fatalf("matching draws: %s", d)
	}
	// Step 3 is past the trace: not compared.
	if d := traceDrawDiff(want, 2, play(7)); d != "" {
		t.Fatalf("untraced step: %s", d)
	}
	for _, tc := range []struct {
		name   string
		step   int
		bounds []int
		want   string
	}{
		{"other bound", 0, []int{4, 1000000}, "script draw 1 differs:\n  got 4:0\n want 100:0"},
		{"swapped order", 0, []int{1000000, 100}, "script draw 1 differs:\n  got 1000000:0\n want 100:0"},
		{"one draw short", 0, []int{100}, "script draw 2 missing:\n want 1000000:0"},
		{"one draw more", 1, []int{1000000, 2}, "script draw 2 not expected:\n  got 2:0"},
		{"draw in another step", 1, nil, "script draw 1 missing:\n want 1000000:0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if d := traceDrawDiff(want, tc.step, play(tc.bounds...)); d != tc.want {
				t.Fatalf("diff = %q, want %q", d, tc.want)
			}
		})
	}
}

// A trace that pins draws needs the random line that makes the run pin and
// record its own.
func TestCheckTraceDrawsNeedARandomLine(t *testing.T) {
	_, _, err := checkTrace(traceScenario(t, "kill wolf\n  S ActionFailed\nkill wolf\n  S ActionFailed\n"), []byte(drawsTrace), nil)
	if err == nil || !strings.Contains(err.Error(), "the trace pins script draws and the scenario has no random line") {
		t.Fatalf("checkTrace error = %v", err)
	}
}

func TestReadTraceRejectsAMalformedDraw(t *testing.T) {
	for _, line := range []string{"# draw 100 Quest", "# draw x:0 Quest"} {
		trace := "# player object 500\n# kill 20120 (object 700)\n" + line + "\n"
		if _, _, err := checkTrace(traceScenario(t, "random always 0\nkill wolf\n"), []byte(trace), nil); err == nil || !strings.Contains(err.Error(), "is not a draw") {
			t.Fatalf("%s: checkTrace error = %v", line, err)
		}
	}
	trace := "# player object 500\n# draw 100:0 Quest\n# kill 20120 (object 700)\n"
	if _, _, err := checkTrace(traceScenario(t, "random always 0\nkill wolf\n"), []byte(trace), nil); err == nil || !strings.Contains(err.Error(), "before the first client packet") {
		t.Fatalf("draw before a step: checkTrace error = %v", err)
	}
}
