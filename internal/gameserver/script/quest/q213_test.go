package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ213MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The class ids the trial is
// open to are named constants in the reference.
func TestQ213MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q213", []string{"quest.Q213_TrialOfTheSeeker"},
		"number 22")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ213PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ213PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q213", "quest/Q213_TrialOfTheSeeker")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
