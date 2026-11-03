package scenario

import (
	"strings"
	"testing"
)

const makerStepsScenario = `character Hunter 20
npcmaker boss_maker sailren_maker max 2 spawn 29065 GrandBoss -113091 -243942 -15536 respawn 30 db sailren corpse 2
npcmaker other_maker default_maker

makerevent boss_maker 11042 0 0
  live 29065 1

killspawn boss_maker
  live 29065 0
`

const makerStepsTrace = `# test trace
# player object 500; format: README.md in this directory.
# makerevent boss_maker 11042 0 0
# heard 29065 (object 700) 11042 1 0
# live 29065 1
# kill 29065 (object 700)
T 500
# maker other_maker 11045 0 0
# live 29065 0
`

// A makerevent step and a killspawn step replay the trace's own lines: the
// maker event as the scenario writes it, the kill by the template of the
// maker's spawn.
func TestCheckTraceReplaysMakerSteps(t *testing.T) {
	sc, err := Parse("maker_steps.scenario", []byte(makerStepsScenario))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := checkTrace(sc, []byte(makerStepsTrace), nil); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct{ trace, want string }{
		"other event": {strings.Replace(makerStepsTrace, "11042 0 0", "11043 0 0", 1), `the step sends "makerevent boss_maker 11042 0 0"`},
		"other maker": {strings.Replace(makerStepsTrace, "makerevent boss_maker", "makerevent other_maker", 1), `the step sends "makerevent boss_maker 11042 0 0"`},
		"other kill":  {strings.Replace(makerStepsTrace, "kill 29065", "kill 29066", 1), `the step sends "kill 29065"`},
	} {
		if _, _, err := checkTrace(sc, []byte(c.trace), nil); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want one containing %q", name, err, c.want)
		}
	}
}

// The heard event of a role-less maker NPC is written with its template
// id; the maker's own send is not a maker line of the step.
func TestCheckTraceNamesAMakerNPCByTemplate(t *testing.T) {
	sc, err := Parse("maker_steps.scenario", []byte(makerStepsScenario))
	if err != nil {
		t.Fatal(err)
	}
	_, seen, err := checkTrace(sc, []byte(makerStepsTrace), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := traceHeardDiff(seen, 0, []string{"29065 11042 1 0"}); got != "" {
		t.Errorf("heard by template id: %s", got)
	}
	if got := traceMakerDiff(seen, 0, nil); got != "" {
		t.Errorf("the makerevent step's own send is a maker line: %s", got)
	}
	if got := traceMakerDiff(seen, 1, []string{"other_maker 11045 0 0"}); got != "" {
		t.Errorf("killspawn's maker line: %s", got)
	}
}

func TestParseRejectsBadMakerSteps(t *testing.T) {
	head := "character A 1\nnpcmaker boss_maker sailren_maker spawn 29065 GrandBoss 0 0 0 corpse 2\nnpcmaker bare_maker default_maker\nnpcmaker no_corpse sailren_maker spawn 29066 GrandBoss 0 0 0\n"
	for name, c := range map[string]struct{ step, want string }{
		"makerevent unknown maker": {"makerevent nobody 1 0 0\n", "no npcmaker line names it"},
		"makerevent bad integer":   {"makerevent boss_maker 1 x 0\n", "makerevent: number"},
		"killspawn no spawn":       {"killspawn bare_maker\n", "no npcmaker line with that name has a spawn with a corpse time"},
		"killspawn no corpse time": {"killspawn no_corpse\n", "no npcmaker line with that name has a spawn with a corpse time"},
		"killspawn unknown":        {"killspawn nobody\n", "no npcmaker line with that name has a spawn with a corpse time"},
	} {
		if _, err := Parse("bad.scenario", []byte(head+c.step)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want one containing %q", name, err, c.want)
		}
	}
	if _, err := Parse("ok.scenario", []byte(head+"makerevent BOSS_MAKER 1 0 0\nkillspawn boss_maker\n")); err != nil {
		t.Errorf("good steps: %v", err)
	}
}

func TestParseReadsAMakerSpawnCorpseTime(t *testing.T) {
	sc, err := Parse("maker_steps.scenario", []byte(makerStepsScenario))
	if err != nil {
		t.Fatal(err)
	}
	if got := sc.NpcMakers[0].Spawn.Corpse; got != 2 {
		t.Errorf("corpse time = %d, want 2", got)
	}
	if !sc.hasCorpses() {
		t.Error("a maker spawn with a corpse time decays no corpses")
	}
}
