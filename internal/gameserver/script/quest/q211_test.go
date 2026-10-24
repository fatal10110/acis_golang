package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ211MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The class ids the trial is
// open to are named constants in the reference.
func TestQ211MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q211", []string{"quest.Q211_TrialOfTheChallenger"},
		"number 19", "number 32", "number 45", "number 47")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ211PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ211PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q211", "quest/Q211_TrialOfTheChallenger")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
