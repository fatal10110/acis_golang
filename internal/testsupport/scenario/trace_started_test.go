package scenario

import (
	"strings"
	"testing"
)

const startedScenario = `character Leader 40
npcmaker boss_maker sailren_maker spawn 29065 GrandBoss -113091 -243942 -15536 db sailren
npcmaker a_maker default_maker
npcmaker b_maker default_maker
started a_maker 11047 0 0
started b_maker 11047 0 0

advance 1s
  live 29065 1
`

const startedTrace = `# test trace
# player object 500; format: README.md in this directory.
# started a_maker 11047 0 0
# started b_maker 11047 0 0
# advance 1000
T 500
# live 29065 1
`

// The script events the makers sent each other as the reference booted
// them are the scenario's started lines, in order.
func TestCheckTraceComparesTheMakersStartedLines(t *testing.T) {
	sc, err := Parse("started.scenario", []byte(startedScenario))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := checkTrace(sc, []byte(startedTrace), nil); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct{ trace, want string }{
		"missing":   {strings.Replace(startedTrace, "# started b_maker 11047 0 0\n", "", 1), "started 2 not expected"},
		"extra":     {strings.Replace(startedTrace, "# advance", "# started c_maker 11047 0 0\n# advance", 1), "started 3 missing"},
		"reordered": {strings.NewReplacer("a_maker", "b_maker", "b_maker", "a_maker").Replace(startedTrace), "started 1 differs"},
		"malformed": {strings.Replace(startedTrace, "11047 0 0", "11047 0", 1), "is not a started"},
		"late":      {strings.Replace(startedTrace, "# live", "# started c_maker 1 0 0\n# live", 1), "after the first client packet"},
	} {
		if _, _, err := checkTrace(sc, []byte(c.trace), nil); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want one containing %q", name, err, c.want)
		}
	}
}

// A scenario with no started line takes a trace with none.
func TestCheckTraceTakesNoStartedLineWithoutOne(t *testing.T) {
	sc, err := Parse("started.scenario", []byte(strings.ReplaceAll(startedScenario, "started a_maker 11047 0 0\nstarted b_maker 11047 0 0\n", "")))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := checkTrace(sc, []byte(startedTrace), nil); err == nil || !strings.Contains(err.Error(), "started 1 missing") {
		t.Errorf("err = %v, want one for the trace's started line", err)
	}
}
