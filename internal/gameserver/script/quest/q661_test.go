package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ661MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ661MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q661", []string{"quest.Q661_MakingTheHarvestGroundsSafe"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ661PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ661PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q661", "quest/Q661_MakingTheHarvestGroundsSafe")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
