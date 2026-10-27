package scenario

import (
	"strings"
	"testing"
)

// mateTrace is a trace of a character and its online mate: the mate's
// packets are the S@Chief lines and its client packets the C@Chief ones, an
// untagged line is the character's.
const mateTrace = `# test trace
# player object 500; format: README.md in this directory.
# mate Chief object 501
# npc darin 30048 (object 600)
C@Chief Action 30048 (object 600)
S@Chief ActionFailed 25
S ActionFailed 25
T 500
C Action 30048 (object 600)
S ActionFailed 25
T 500
# away
S@Chief ActionFailed 25
C@Chief RequestBypassToServer npc_600_Quest
S@Chief ActionFailed 25
T 500
`

// mateSteps replays mateTrace.
const mateSteps = `as Chief action darin
  S ActionFailed
  to player ActionFailed
action darin
  S ActionFailed
away
  to Chief ActionFailed
as Chief bypass npc_{darin}_Quest
  S ActionFailed
`

func mateScenario(t *testing.T, steps string) *Scenario {
	t.Helper()
	sc, err := Parse("traced.scenario", []byte("trace x.golden\ncharacter A 1\nclan 3\nmate Chief 19 leader sponsor online\nnpc darin 30048 Folk 0 0 0\n"+steps))
	if err != nil {
		t.Fatal(err)
	}
	return sc
}

// A mate's client packets and the packets it was sent are told from the
// character's by their tag, and each client's are compared with the step's S
// lines (the client that played it) or its to line.
func TestCheckTraceReplaysAMatesClient(t *testing.T) {
	if _, err := checkTrace(mateScenario(t, mateSteps), []byte(mateTrace), nil); err != nil {
		t.Fatal(err)
	}
}

func TestCheckTraceNamesADepartureOfAMate(t *testing.T) {
	for _, tc := range []struct{ name, from, to, want string }{
		{"the mate's packet", "as Chief action darin\n  S ActionFailed\n", "as Chief action darin\n", `against trace "C@Chief Action 30048 (object 600)": scenario packet 1 missing`},
		{"the character's packet in the mate's step", "  to player ActionFailed\n", "", "scenario packet to player 1 missing"},
		{"a packet the mate was not sent", "  to Chief ActionFailed\n", "", "scenario packet to Chief 1 missing"},
		{"the mate's client", "as Chief action darin", "action darin", `the step sends "C Action 30048 (object 600)"`},
		{"the character's client", "\naction darin\n", "\nas Chief action darin\n", `the step sends "C@Chief Action 30048 (object 600)"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			steps := strings.Replace(mateSteps, tc.from, tc.to, 1)
			_, err := checkTrace(mateScenario(t, steps), []byte(mateTrace), nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("checkTrace error = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

// A mate that enters the world selects its character and enters on its own
// client; the burst it is sent is not compared, what the character is sent is.
func TestCheckTraceReplaysAMateEnteringTheWorld(t *testing.T) {
	const trace = `# test trace
# player object 500; format: README.md in this directory.
# mate Chief object 501
C@Chief RequestGameStart 0
T 500
C@Chief EnterWorld
S@Chief ActionFailed 25
S ActionFailed 25
T 500
`
	sc, err := Parse("traced.scenario", []byte("trace x.golden\ncharacter A 1\nclan 3\nmate Chief 19 leader sponsor\nenter Chief\n  S ActionFailed\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := checkTrace(sc, []byte(trace), nil); err != nil {
		t.Fatal(err)
	}
	sc.Steps[0].Packets = nil
	if _, err := checkTrace(sc, []byte(trace), nil); err == nil || !strings.Contains(err.Error(), "scenario packet 1 missing") {
		t.Fatalf("checkTrace error = %v, want the character's missing packet", err)
	}
}

func TestParseTracedScenarioTakesAMatesSteps(t *testing.T) {
	for _, steps := range []string{"mate Chief 19 leader sponsor online\nnpc darin 30048 Folk 0 0 0\nas Chief action darin\n", "mate Chief 19 leader apprentice\nenter Chief\n"} {
		if _, err := Parse("traced.scenario", []byte("trace x.golden\ncharacter A 1\nclan 3\n"+steps)); err != nil {
			t.Errorf("%q: %v", steps, err)
		}
	}
}
