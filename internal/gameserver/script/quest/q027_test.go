package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ027MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ027MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q027", []string{"quest.Q027_ChestCaughtWithABaitOfWind"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ027PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ027PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q027", "quest/Q027_ChestCaughtWithABaitOfWind")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
