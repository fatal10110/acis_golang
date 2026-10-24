package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ028MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ028MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q028", []string{"quest.Q028_ChestCaughtWithABaitOfIcyAir"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ028PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ028PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q028", "quest/Q028_ChestCaughtWithABaitOfIcyAir")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
