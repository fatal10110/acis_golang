package scenario

import (
	"slices"
	"strings"
	"testing"
)

const makerTrace = `# test trace
# player object 500; format: README.md in this directory.
# event 20120 (object 700) 7 character 0
# maker room_a 1000 0 0
# maker ROOM_A 1001 -5 7
S ActionFailed 25
`

// The npcmaker script events of a step are its maker lines, in the
// trace's order.
func TestCheckTraceReadsTheEventsNpcMakersWereSent(t *testing.T) {
	sc := traceScenario(t, "event wolf 7 character 0\n  S ActionFailed\n")
	_, seen, err := checkTrace(sc, []byte(makerTrace), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"room_a 1000 0 0", "ROOM_A 1001 -5 7"}
	if len(seen) != 1 || !slices.Equal(seen[0].makers, want) {
		t.Errorf("makers = %q, want %q", seen, want)
	}
}

// A maker line is well formed and follows a client packet of the trace.
func TestCheckTraceRejectsABadMakerLine(t *testing.T) {
	for name, c := range map[string]struct{ trace, want string }{
		"malformed": {strings.Replace(makerTrace, "1000 0 0", "1000 0", 1), "is not a maker"},
		"first":     {strings.Replace(makerTrace, "# event", "# maker a 1 0 0\n# event", 1), "before the first client packet"},
	} {
		_, _, err := checkTrace(traceScenario(t, "event wolf 7 character 0\n  S ActionFailed\n"), []byte(c.trace), nil)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want one containing %q", name, err, c.want)
		}
	}
}

// The events the makers were sent in a step are compared in order: a
// maker told to delete then spawn is not one told to spawn then delete.
func TestTraceMakerDiff(t *testing.T) {
	trace := []traceSeen{{makers: []string{"a 1000 0 0", "a 1001 0 0"}}}
	if d := traceMakerDiff(trace, 0, []string{"a 1000 0 0", "a 1001 0 0"}); d != "" {
		t.Errorf("the same events differ: %s", d)
	}
	if d := traceMakerDiff(trace, 0, []string{"a 1001 0 0", "a 1000 0 0"}); d == "" {
		t.Error("the events in another order do not differ")
	}
	if d := traceMakerDiff(trace, 0, nil); d == "" {
		t.Error("a missing event does not differ")
	}
	if d := traceMakerDiff(trace, 1, []string{"a 1000 0 0"}); d != "" {
		t.Errorf("a step past the trace is compared: %s", d)
	}
}

// An npcmaker line names a maker and its AI type, once per name; a corpse
// option is a number of seconds, once.
func TestParseNpcMakerAndCorpse(t *testing.T) {
	sc, err := Parse("ok.scenario", []byte("character A 1\nnpcmaker Room_A royal_rush_maker\nnpc a 1 Monster 0 0 0 corpse 3\nadvance 1s\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(sc.NpcMakers) != 1 || sc.NpcMakers[0] != (NpcMaker{Name: "Room_A", AIType: "royal_rush_maker"}) || sc.NPCs[0].Corpse != 3 {
		t.Errorf("npcmakers = %v, corpse = %d", sc.NpcMakers, sc.NPCs[0].Corpse)
	}
	for name, c := range map[string]struct{ text, want string }{
		"maker args":   {"character A 1\nnpcmaker a\n", "npcmaker <name> <ai type>"},
		"maker twice":  {"character A 1\nnpcmaker a x\nnpcmaker A y\n", "listed twice"},
		"corpse nan":   {"character A 1\nnpc a 1 Monster 0 0 0 corpse x\nadvance 1s\n", "corpse time"},
		"corpse zero":  {"character A 1\nnpc a 1 Monster 0 0 0 corpse 0\nadvance 1s\n", "corpse time"},
		"corpse twice": {"character A 1\nnpc a 1 Monster 0 0 0 corpse 1 corpse 2\nadvance 1s\n", "corpse <seconds>"},
	} {
		if _, err := Parse("bad.scenario", []byte(c.text)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want one containing %q", name, err, c.want)
		}
	}
}
