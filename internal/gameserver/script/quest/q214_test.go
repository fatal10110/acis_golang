package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ214MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The dark wizard's class id is
// a named constant in the reference; the human and elven wizards' ids are
// also cond values it spells out.
func TestQ214MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q214", []string{"quest.Q214_TrialOfTheScholar"},
		"number 39")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ214PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ214PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q214", "quest/Q214_TrialOfTheScholar")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
