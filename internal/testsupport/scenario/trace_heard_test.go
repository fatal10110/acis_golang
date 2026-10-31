package scenario

import (
	"slices"
	"strings"
	"testing"
)

// heardTrace is a reference trace of an event step whose wolf answered with
// a broadcast: the wolf itself, a second NPC the trace never named by a
// line (20121) and the character heard it.
const heardTrace = `# test trace
# player object 500; format: README.md in this directory.
# event 20120 (object 700) 7 character 0
# heard 20120 (object 700) 9 500 0
# heard 20121 (object 800) 9 700 -3
S ActionFailed 25
`

// The NPCs a script's send reached are the step's heard lines: the
// receiver and the object ids among the arguments named by role.
func TestCheckTraceReadsTheEventsNPCsHeard(t *testing.T) {
	sc := traceScenario(t, "event wolf 7 character 0\n  S ActionFailed\n")
	_, heard, err := checkTrace(sc, []byte(heardTrace), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"wolf 9 player 0", "20121 9 wolf -3"}}
	if len(heard) != 1 || !slices.Equal(heard[0], want[0]) {
		t.Errorf("heard = %q, want %q", heard, want)
	}
}

// A heard line is well formed and follows a client packet of the trace.
func TestCheckTraceRejectsABadHeardLine(t *testing.T) {
	for name, c := range map[string]struct{ trace, want string }{
		"malformed": {strings.Replace(heardTrace, "9 500 0", "9 500", 1), "is not a heard"},
		"first":     {strings.Replace(heardTrace, "# event", "# heard 20120 (object 700) 9 500 0\n# event", 1), "before the first client packet"},
	} {
		_, _, err := checkTrace(traceScenario(t, "event wolf 7 character 0\n  S ActionFailed\n"), []byte(c.trace), nil)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: checkTrace error = %v, want one containing %q", name, err, c.want)
		}
	}
}

// The events heard in a step are compared as a set: the order the NPCs
// around a broadcast hear it in is not the reference's.
func TestTraceHeardDiff(t *testing.T) {
	trace := [][]string{{"a 1 0 0", "b 1 0 0"}}
	if d := traceHeardDiff(trace, 0, []string{"b 1 0 0", "a 1 0 0"}); d != "" {
		t.Errorf("same events in another order differ: %s", d)
	}
	if d := traceHeardDiff(trace, 0, []string{"a 1 0 0"}); d == "" {
		t.Error("a missing event does not differ")
	}
	if d := traceHeardDiff(trace, 1, []string{"a 1 0 0"}); d != "" {
		t.Errorf("a step past the trace is compared: %s", d)
	}
}
