package q215

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The class ids the trial is
// open to are named constants in the reference.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{"quest.Q215_TrialOfThePilgrim"},
		"number 29", "number 42", "number 50")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestPagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, ".", "quest/Q215_TrialOfThePilgrim")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
