package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ021MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The NpcString ids of the
// NPCs' lines are named constants in the reference.
func TestQ021MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q021", []string{"quest.Q021_HiddenTruth"},
		"number 2150", "number 2151", "number 2152")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ021PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ021PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q021", "quest/Q021_HiddenTruth")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
