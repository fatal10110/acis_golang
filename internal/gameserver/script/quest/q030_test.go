package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ030MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ030MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q030", []string{"quest.Q030_ChestCaughtWithABaitOfFire"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ030PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ030PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q030", "quest/Q030_ChestCaughtWithABaitOfFire")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
