package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ403MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The NpcString ids of the bandit's two lines and the
// class id of the rogue are named constants in the reference.
func TestQ403MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q403", []string{"quest.Q403_PathToARogue"}, "number 40306", "number 40307", "number 7")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ403PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ403PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q403", "quest/Q403_PathToARogue")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
