package scenario

import (
	"strings"
	"testing"
)

// flowTrace is a scenario trace of a created character: its creation, its
// first entry, a question mark, a pickup, an approach, two AI ticks and a
// relog, whose packets the run does not compare.
const flowTrace = `# test trace
# player object 500; format: README.md in this directory.
C RequestCharacterCreate A
Q INSERT INTO character_quests (charId,name,var,value) VALUES (?,?,?,?) ON DUPLICATE KEY UPDATE value=VALUES(value) | 500 | Tutorial | <state> | STARTED
T 500
C RequestGameStart 0
Q SELECT name,var,value FROM character_quests WHERE charId=? | 500
T 500
C EnterWorld
S ActionFailed 25
T 500
C RequestTutorialQuestionMark 1
S ActionFailed 25
T 500
C Action item 6353 (object 900)
S ActionFailed 25
T 500
# approach 20120 (object 700)
S ActionFailed 25
# tick
# tick
S ActionFailed 25
C RequestRestart
S ActionFailed 25
T 500
C RequestGameStart 0
T 500
C EnterWorld
S ActionFailed 25
Q INSERT INTO character_quests (charId,name,var,value) VALUES (?,?,?,?) ON DUPLICATE KEY UPDATE value=VALUES(value) | 500 | Tutorial | Ex | 1
T 500
`

// flowSteps replays flowTrace.
const flowSteps = `create
  Q upsert Tutorial | <state> | STARTED
enter
  S ActionFailed
mark 1
  S ActionFailed
pickup 6353
  S ActionFailed
approach wolf
  S ActionFailed
tick x2
  S ActionFailed
relog
  Q upsert Tutorial | Ex | 1
`

// Every step verb replays its client packets or probe actions: a relog is
// the trace's restart, character select and enter world, whose packets are
// not compared, an enter its character select and enter world.
func TestCheckTraceReplaysEveryStepVerb(t *testing.T) {
	if _, err := checkTrace(traceScenario(t, flowSteps), []byte(flowTrace), nil); err != nil {
		t.Fatal(err)
	}
}

func TestCheckTraceNamesADepartureOfACreatedCharacter(t *testing.T) {
	for _, tc := range []struct{ name, from, to, want string }{
		{"other mark", "mark 1", "mark 2", `the step sends "C RequestTutorialQuestionMark 2"`},
		{"other pickup", "pickup 6353", "pickup 57", `the step sends "pickup 57"`},
		{"fewer ticks", "tick x2\n  S ActionFailed\n", "tick\n", `step 7 (line 15) "relog" against trace "tick": the step sends "C RequestRestart"`},
		{"restart for relog", "relog\n  Q upsert Tutorial | Ex | 1\n", "restart\n  S ActionFailed\n", `the trace has 11 client packets, the scenario 7 steps`},
		{"relog statement", "  Q upsert Tutorial | Ex | 1\n", "", "scenario statement 1 missing"},
		{"enter packets", "enter\n  S ActionFailed\n", "enter\n", "scenario packet 1 missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			steps := strings.Replace(flowSteps, tc.from, tc.to, 1)
			_, err := checkTrace(traceScenario(t, steps), []byte(flowTrace), nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("checkTrace error = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

// The probe plays every step of a scenario it traces: its trace must cover
// them all, while another trace may cover the first steps only.
func TestCheckTraceRequiresAProbeTraceToCoverEveryStep(t *testing.T) {
	sc := traceScenario(t, flowSteps+"advance 1s\n")
	if _, err := checkTrace(sc, []byte(flowTrace), nil); err != nil {
		t.Fatalf("a trace of the first steps: %v", err)
	}
	sc.Trace = probeTraces + "flow.golden"
	_, err := checkTrace(sc, []byte(flowTrace), nil)
	if want := `the trace ends before step 8 (line 18) "advance 1s"`; err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("checkTrace error = %v, want one containing %q", err, want)
	}
}
