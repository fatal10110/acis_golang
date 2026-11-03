package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ115MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The NpcString ids of the
// suspicious man's lines are named constants in the reference.
func TestQ115MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q115", []string{"quest.Q115_TheOtherSideOfTruth"},
		"number 11550", "number 11551", "number 11552", "number 11553")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ115PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ115PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q115", "quest/Q115_TheOtherSideOfTruth")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
