package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ216MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The class ids the trial is
// open to are named constants in the reference.
func TestQ216MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q216", []string{"quest.Q216_TrialOfTheGuildsman"},
		"number 54", "number 56")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ216PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ216PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q216", "quest/Q216_TrialOfTheGuildsman")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
