package scenario

import "testing"

// A traced scenario takes the level of an npc a kill step names, the one the
// reference probe plays.
func TestTracedScenarioTakesTheLevelOfAFoughtNPC(t *testing.T) {
	t.Parallel()
	sc, err := Parse("ok.scenario", []byte("trace x.golden\ncharacter A 1\nnpc a 1 Monster 0 0 0 level 20\nkill a\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := sc.NPCs[0].Level; got != 20 {
		t.Errorf("level = %d, want 20", got)
	}
}
