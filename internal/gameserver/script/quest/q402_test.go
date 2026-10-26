package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ402MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The class id of the knight is a named
// constant in the reference.
func TestQ402MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q402", []string{"quest.Q402_PathToAHumanKnight"}, "number 4")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ402PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ402PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q402", "quest/Q402_PathToAHumanKnight")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
