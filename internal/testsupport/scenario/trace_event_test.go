package scenario

import (
	"strings"
	"testing"
)

// eventTrace is a reference trace of an event step: the probe sent the wolf
// its script event 7 naming the character, and the NPC answered.
const eventTrace = `# test trace
# player object 500; format: README.md in this directory.
# event 20120 (object 700) 7 character 0
S ActionFailed 25
`

// An event step replays the trace's event of the NPC, with the arguments as
// the scenario writes them.
func TestCheckTraceReplaysAnEventStep(t *testing.T) {
	sc := traceScenario(t, "event wolf 7 character 0\n  S ActionFailed\n")
	if _, err := checkTrace(sc, []byte(eventTrace), nil); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, step, want string }{
		{"other id", "event wolf 8 character 0\n", `the step sends "event 20120 8 character 0"`},
		{"other argument", "event wolf 7 0 0\n", `the step sends "event 20120 7 0 0"`},
	} {
		_, err := checkTrace(traceScenario(t, c.step+"  S ActionFailed\n"), []byte(eventTrace), nil)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: checkTrace error = %v, want one containing %q", c.name, err, c.want)
		}
	}
}

// A traced scenario may carry a clock line, the ai and params options of an
// npc line and an event step, which the probe plays.
func TestParseTracedClockParamsAndEvent(t *testing.T) {
	text := "trace x.golden\ncharacter A 1\nclock 00:49:57\nnpc a 1 Folk 0 0 0\nnpc b 2 Folk 0 0 0 ai k=v params character a 3\nevent b 7 character 0\n"
	if _, err := Parse("traced.scenario", []byte(text)); err != nil {
		t.Fatal(err)
	}
}
