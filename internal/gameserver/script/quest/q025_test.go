package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ025MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The NpcString ids of the
// NPCs' lines are named constants in the reference.
func TestQ025MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q025", []string{"quest.Q025_HidingBehindTheTruth"},
		"number 2550", "number 2551")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ025PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ025PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q025", "quest/Q025_HidingBehindTheTruth")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
