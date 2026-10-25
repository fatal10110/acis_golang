package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ126MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The reference reads
// Q125's state through Q125's own name constant, so the name is not one of
// its literals.
func TestQ126MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q126", []string{"quest.Q126_TheNameOfEvil_2"}, `string "Q125_TheNameOfEvil_1"`)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ126PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ126PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q126", "quest/Q126_TheNameOfEvil_2")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
