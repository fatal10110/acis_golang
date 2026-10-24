package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ125MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The reference reads
// Q124's state through Q124's own name constant, so the name is not one of
// its literals.
func TestQ125MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q125", []string{"quest.Q125_TheNameOfEvil_1"}, `string "Q124_MeetingTheElroki"`)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ125PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ125PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q125", "quest/Q125_TheNameOfEvil_1")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
