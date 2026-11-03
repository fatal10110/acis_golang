package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ635MatchesReferenceFingerprints compares the package with the
// reference class: literals, engine calls and hook shapes. The reference
// loops its NPC ranges to one past the last id, names the Anakazel room's
// coordinates apart from the others and prints a missing room index as
// null; its created and timer hooks call the parent's, which does nothing.
func TestQ635MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q635", []string{"quest.Q635_InTheDimensionalRift"},
		"number 31508", "number 31919", `string "PosX8"`, `string "PosY8"`, `string "null"`,
		"hook onCreated direct same", "hook onCreated none", "hook onTimer direct same,same", "hook onTimer none", "call Quest.getPartyMembers")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ635PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ635PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q635", "quest/Q635_InTheDimensionalRift")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
